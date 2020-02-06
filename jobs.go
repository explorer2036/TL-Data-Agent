package main

import (
	"TL-Data-Agent/crypto"
	"TL-Data-Agent/entity"
	"TL-Data-Agent/log"
	"TL-Data-Agent/proto/gateway"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	// Rfc3339Milli unified time format, refer to time.RFC3339Nano
	Rfc3339Milli = "2006-01-02T15:04:05.999Z07:00"
)

var (
	// arrary of export types, currently only "data" is supported, in future there might be DOM
	exportTypes = []string{"data"}
)

const (
	// DefaultGRPCTimeout - default timeout for sending grpc messages
	DefaultGRPCTimeout = 5
)

// report data by the grpc connection
func (p *Program) report(v interface{}) error {
	// marshal the value to json
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	// context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*DefaultGRPCTimeout)
	defer cancel()

	// prepare the request
	request := gateway.ReportRequest{
		ApplicationId: p.settings.App.Application,
		LoginId:       p.user.LoginId,
		Token:         p.user.Token,
		Data:          data,
	}
	reply, err := p.serviceClient.Report(ctx, &request)
	if err != nil {
		return err
	}
	if reply.Status != gateway.Status_Success {
		return fmt.Errorf("status: %v, message: %v", reply.Status, reply.Message)
	}

	return nil
}

// login by the encrypted file
func (p *Program) loginByFile() error {
	// read the username and password
	s, err := crypto.DecryptFile(encryptedFile, encryptKey)
	if err != nil {
		return err
	}

	// login with username and password
	parts := strings.Split(string(s), ":")
	reply, err := p.login(parts[0], parts[1])
	if err != nil {
		// mark the login unhealthy
		p.healthy = false
		return err
	}

	// update the user's information
	p.user.LoginId = parts[0]
	p.user.Password = parts[1]
	p.user.Token = reply.Token
	// format the uuid to user id
	p.user.UserID = uuid2id(reply.UserID)

	// mark ready to send messages
	p.ready = true
	p.healthy = true

	return nil
}

func (p *Program) flush(v interface{}) error {
	// send the messages to gateway
	if err := p.report(v); err != nil {
		// check if the token is refused
		if strings.Contains(err.Error(), "status code: 401") {
			// login by the encrypted file
			if ierr := p.loginByFile(); ierr != nil {
				log.Errorf("login by ecnrypted file: %v", ierr)
			}
		}
		return err
	}

	return nil
}

// write a heartbeat message to gateway
func (p *Program) heartbeat() {
	log.Info("heartbeat report - start")

	if p.ready == false {
		log.Warn("hearbeat report - user hasn't login")
		return
	}

	// create the heartbeat data
	hearbeat := entity.Heartbeat{
		Kind:   "data_heartbeat",
		Action: "update",
		UserID: p.user.UserID,
		Source: p.user.UUID,
		Path:   "&&heartbeat",
		Data: entity.HeartbeatData{
			Status: "online",
		},
		Timestamp: time.Now().Format(Rfc3339Milli),
	}

	// try send the data to gateway
	if err := p.flush(&hearbeat); err != nil {
		log.Errorf("send data to gateway, data: %v, error: %v", hearbeat, err)
		return
	}

	log.Info("heartbeat report - end")
}

// whether the slice contains the target string or not
func contains(ss []string, target string) bool {
	b := false
	for _, s := range ss {
		if s == target {
			b = true
			break
		}
	}
	return b
}

// parse the valid data files
func (p *Program) parse(folder string) ([]string, error) {
	// collect the names of json files in data folder
	files, err := ioutil.ReadDir(folder)
	if err != nil {
		return nil, err
	}

	jsons := []string{}
	for _, file := range files {
		name := file.Name()
		if path.Ext(name) == ".json" {
			jsons = append(jsons, name)
		}
	}

	// whether the lock is for all
	lock := false
	// the types that have been locked
	lockTypes := make([]string, 0, len(exportTypes))

	// the other json files
	dataFiles := []string{}
	for _, name := range jsons {
		fn := strings.Split(name, ".")[0]
		// check if it's lock file
		if strings.HasPrefix(fn, "lock") {
			locks := strings.Split(fn, "-")
			if len(locks) == 1 {
				lock = true // lock file
			} else {
				// lock-xx.json
				if len(locks) == 2 {
					l := strings.ToLower(locks[1])
					// lock-data.json
					if contains(exportTypes, l) {
						// append it to lock types only if it is a supported export type
						lockTypes = append(lockTypes, l)
					}
				}
			}
		} else {
			pieces := strings.Split(fn, "_")
			// a valid name should be split into 3 pieces by underscore sign
			// <export>_<app>_<unique timestamp>.json
			if len(pieces) == 3 {
				dataFiles = append(dataFiles, name)
			}
		}
	}
	if lock == true {
		log.Info("lock.json is in the data folder")
		return nil, nil

	}

	// get valid json files for processing, filter by export type and lock
	vfs := []string{}
	// <export>_<app>_<unique timestamp>.json
	for _, f := range dataFiles {
		// convert to lower case
		export := strings.ToLower(strings.Split(f, "_")[0])
		if !contains(exportTypes, export) {
			log.Infof("%s is skipped. It's export type is not supported", f)
		} else if contains(lockTypes, export) {
			log.Infof("%s is skipped. It's export type is locked", f)
		} else {
			// if the export is a supported type and not locked by lock-xx.json, then append it to valid file list
			vfs = append(vfs, f)
		}
	}

	return vfs, nil
}

func validate(m *entity.Message) error {
	if m.Kind == "" {
		return errors.New("dtype is empty")
	}
	if len(m.Data) == 0 {
		return errors.New("data is empty")
	}

	switch m.Action {
	case "insert", "update":
		if m.Source == "" {
			return errors.New("source is empty")
		}
		if m.Path == "" {
			return errors.New("path is empty")
		}
		if m.Time == "" {
			return errors.New("time is empty")
		}
	case "add", "del":
		// nothing special fields to check
	default:
		return fmt.Errorf("invalid action: %v", m.Action)
	}

	return nil
}

// transfer the data
func (p *Program) transfer(name string, data []byte) error {
	var msgs []entity.Message

	// unmarshal the bytes to message structure
	if err := json.Unmarshal(data, &msgs); err != nil {
		return err
	}

	for index, msg := range msgs {
		// check whether data field is a valid json string
		if err := validate(&msg); err != nil {
			log.Errorf("message is invalid, file: %s index: %d, data: %s, error: %v", name, index, msg, err)
			continue
		}

		// these two fields are provided by collector
		msg.UserID = p.user.UserID
		msg.Timestamp = time.Now().Format(Rfc3339Milli)

		for {
			// try send the data to gateway
			if err := p.flush(&msg); err != nil {
				log.Errorf("send data to gateway, file: %s index: %d, data: %v, error: %v", name, index, msg, err)

				// retry again until sending success
				time.Sleep(2 * time.Second)
				continue
			}

			break
		}
	}

	return nil
}

// process the data file
func (p *Program) process(folder string, name string) {
	// concatenate data folder and json file name
	fullpath := folder + "\\" + name
	path, err := syscall.UTF16PtrFromString(fullpath)
	if err != nil {
		return
	}

	// try to open this file exclusively by windows api, see https://golang.org/src/syscall/syscall_windows.go#L248
	handle, err := windows.CreateFile(path, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		log.Infof("%s in use by another process", name)
	} else {
		log.Infof("%s can be opened in exclusive mode", name)

		var done uint32
		data := make([]byte, maxFileSize)
		// read from the file
		windows.ReadFile(handle, data, &done, nil)
		// trim slice to the number of bytes that have been read
		data = data[:done]

		// transfer the messages to gateway
		if err := p.transfer(name, data); err != nil {
			log.Errorf("error occurs in processing file %s: %v", name, err)
		}

		// close the file first
		windows.CloseHandle(handle)
		// delete the json file
		windows.DeleteFile(path)
	}
}

// collecting messages from data folder and sending to gateway
func (p *Program) collect() {
	log.Info("data collecting - start")

	if p.ready == false {
		log.Warn("data collecting - user hasn't login")
		return
	}

	// parse the data folder path
	folder, err := ioutil.ReadFile(p.settings.App.BaseDir + dataPath)
	if err != nil {
		log.Errorf("read the file %s: %v", dataPath, err)
		return
	}

	// parse the valid data files
	files, err := p.parse(string(folder))
	if err != nil {
		log.Errorf("parse to retrieve files: %v", err)
		return
	}
	if len(files) == 0 {
		log.Info("no files to process")
		return
	}

	// handle the data files one bye one
	for _, file := range files {
		p.process(string(folder), file)
	}

	// handle the files
	log.Info("data collecting - end")
}
