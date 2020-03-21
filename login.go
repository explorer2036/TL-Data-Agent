package main

import (
	"TL-Data-Agent/crypto"
	"TL-Data-Agent/log"
	"TL-Data-Agent/proto/gateway"
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emicklei/go-restful"
)

var (
	encryptKey    = "zYUTfA6Sa1lxTA43"
	encryptedFile = "login.dat"
)

// User for login id and password
type User struct {
	UserID   int
	LoginId  string
	Password string
	Token    string
	UUID     string
	Healthy  bool
}

// Health - Check if the login status is healthy
func (p *Program) Health(request *restful.Request, response *restful.Response) {
	if p.healthy {
		response.WriteEntity("healthy")
	} else {
		response.WriteEntity("unhealthy")
	}
}

func (p *Program) register(loginId string, password string) (*gateway.RegisterReply, error) {
	// context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*DefaultGRPCTimeout)
	defer cancel()

	registerRequest := &gateway.RegisterRequest{
		LoginId:       loginId,
		Password:      password,
		ApplicationId: p.settings.App.Application,
	}
	registerReply, err := p.serviceClient.Register(ctx, registerRequest)
	if err != nil {
		return nil, err
	}
	return registerReply, nil
}

// Register with login id and password
func (p *Program) Register(request *restful.Request, response *restful.Response) {
	// read the request body
	user := new(User)
	if err := request.ReadEntity(&user); err != nil {
		response.WriteError(http.StatusInternalServerError, err)
		return
	}

	// register with the login id and password
	reply, err := p.register(user.LoginId, user.Password)
	if err != nil {
		response.WriteError(http.StatusInternalServerError, err)
		return
	}
	if reply.Status != gateway.Status_Success {
		response.WriteErrorString(http.StatusInternalServerError, reply.Message)
		return
	}

	response.WriteHeader(http.StatusOK)
}

func (p *Program) login(loginId string, password string) (*gateway.LoginReply, error) {
	// context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*DefaultGRPCTimeout)
	defer cancel()

	loginRequest := &gateway.LoginRequest{
		LoginId:       loginId,
		Password:      password,
		ApplicationId: p.settings.App.Application,
	}
	// login with the id and password by gateway
	loginReply, err := p.serviceClient.Login(ctx, loginRequest)
	if err != nil {
		return nil, err
	}

	return loginReply, nil
}

// Login with the login id and password
func (p *Program) Login(request *restful.Request, response *restful.Response) {
	user := new(User)
	if err := request.ReadEntity(&user); err != nil {
		response.WriteError(http.StatusInternalServerError, err)
		return
	}

	// login with the login id and password
	reply, err := p.login(user.LoginId, user.Password)
	if err != nil {
		response.WriteError(http.StatusInternalServerError, err)
		return
	}
	if reply.Status != gateway.Status_Success {
		response.WriteErrorString(http.StatusInternalServerError, reply.Message)
		return
	}

	content := user.LoginId + ":" + user.Password
	// write the login id and password to file
	if err := crypto.EncryptFile(encryptedFile, []byte(content), encryptKey); err != nil {
		response.WriteError(http.StatusInternalServerError, err)
		return
	}

	// update the user's information
	p.user.LoginId = user.LoginId
	p.user.Password = user.Password
	p.user.Token = reply.Token
	// format the uuid to user id
	p.user.UserID = uuid2id(reply.UserID)
	p.user.UUID = reply.Source
	log.Infof("login response: %v", reply)

	// mark ready to send messages
	p.ready = true
	p.healthy = true

	response.WriteHeader(http.StatusOK)
}

// dbd62208-d0ea-4a5a-9066-64d41e7dcedd
// 8+4+4+4+12
// for the uuid format, i will put the user id with 10 bytes replace the part "d0ea-4a5a-90"
func uuid2id(uuid string) int {
	if len(uuid) != 36 {
		panic(uuid)
	}

	out := strings.Replace(uuid[9:21], "-", "", -1)
	id, err := strconv.Atoi(out)
	if err != nil {
		panic(err)
	}
	return id
}
