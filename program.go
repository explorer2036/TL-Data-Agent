package main

import (
	"TL-Data-Agent/config"
	"TL-Data-Agent/log"
	"TL-Data-Agent/proto/gateway"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/emicklei/go-restful"
	"github.com/kardianos/service"
	"golang.org/x/sys/windows"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

var (
	// assume that the file size won't exceed 1MB
	maxFileSize = 1024 * 1024
	// data file path
	dataPath = "\\conf\\appdata.txt"
)

const (
	// GRPCClientDialTimeout - the timeout for client to dial the server
	GRPCClientDialTimeout = 5
	// GRPCClientKeepaliveTime - After a duration of this time if the client doesn't see any activity it
	// pings the server to see if the transport is still alive.
	GRPCClientKeepaliveTime = 15
	// GRPCClientKeepaliveTimeout - After having pinged for keepalive check, the client waits for a duration
	// of Timeout and if no activity is seen even after that the connection is closed.
	GRPCClientKeepaliveTimeout = 5
)

// Program define Start and Stop methods.
type Program struct {
	lockFile      string                // lock file name
	settings      *config.Config        // the settings for program
	exit          chan struct{}         // exit signal
	conn          *grpc.ClientConn      // grpc connection
	serviceClient gateway.ServiceClient // gateway service client
	httpServer    *http.Server          // http server for login ui

	user  User   // the user's information for authentization
	ready bool   // whether it's ready to send messages to gateway
	uuid  string // uuid for user's machine

	healthy bool // whether it's healthy for login
}

// check if the path exists or not
func exists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

// prepare the files for certification
func (p *Program) createCredentials() credentials.TransportCredentials {
	cert, err := tls.LoadX509KeyPair(p.settings.TLS.Perm, p.settings.TLS.Key)
	if err != nil {
		panic(err)
	}

	ca, err := ioutil.ReadFile(p.settings.TLS.Ca)
	if err != nil {
		panic(err)
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ca); !ok {
		panic("append certs from pem")
	}

	tlsConfig := &tls.Config{}
	tlsConfig.RootCAs = certPool
	tlsConfig.Certificates = []tls.Certificate{cert}
	tlsConfig.BuildNameToCertificate()

	return credentials.NewTLS(tlsConfig)
}

// init the service client
func (p *Program) initServiceClient() {
	// prepare the dial options for grpc client
	opts := []grpc.DialOption{}
	opts = append(opts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:    GRPCClientKeepaliveTime * time.Second,
		Timeout: GRPCClientKeepaliveTimeout * time.Second,
	}))
	if p.settings.TLS.Switch {
		// prepare the credentials with the ca files
		creds := p.createCredentials()
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	// set up a connection to the server.
	conn, err := grpc.Dial(p.settings.App.Gateway, opts...)
	if err != nil {
		panic(fmt.Sprintf("grpc conn: %v", err))
	}
	// update the connection
	p.conn = conn

	// new service clint
	p.serviceClient = gateway.NewServiceClient(conn)
}

// NewProgram create a program for crob jobs
func NewProgram(settings *config.Config) *Program {
	program := &Program{
		settings: settings,
		exit:     make(chan struct{}),
	}

	// init the grpc connection and service client
	program.initServiceClient()

	// lock file name
	program.lockFile = settings.App.BaseDir + "\\tmp.lock"

	return program
}

// Start the service
func (p *Program) Start(s service.Service) error {
	if service.Interactive() {
		log.Info("Running in terminal.")
	} else {
		log.Info("Running under service manager.")
	}

	// do the actual work async.
	go p.run()

	return nil
}

// start a cron job
func startCronJob(ctx context.Context, wg *sync.WaitGroup, interval int, f func()) {
	defer wg.Done()

	// new a timer for heartbeat
	ticker := time.NewTimer(time.Second * time.Duration(interval))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			f()
		case <-ctx.Done():
			log.Info("cron job goroutine is exited")
			return
		}

		// reset the timer
		ticker.Reset(time.Second * time.Duration(interval))
	}
}

// newContainer returns a restful container with routes
func (p *Program) newContainer() *restful.Container {
	container := restful.NewContainer()

	ws := new(restful.WebService)
	ws.Path("/").Doc("root").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML)

	ws.Route(ws.POST("/login").To(p.Login))       // user login route
	ws.Route(ws.POST("/register").To(p.Register)) // user register route
	ws.Route(ws.POST("/health").To(p.Health))     // health route

	container.Add(ws)

	return container
}

// start the http server
func (p *Program) startServer(wg *sync.WaitGroup) {
	// start the http server
	p.httpServer = &http.Server{
		Addr:    p.settings.App.Server,
		Handler: p.newContainer(),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := p.httpServer.ListenAndServe(); err != nil {
			log.Info("http server is closed")
			return
		}
	}()
}

// run the service logic
func (p *Program) run() error {
	// check if the program is already running
	handle, err := p.hasLockfile()
	if err != nil {
		log.Error("A collector process is already running")
		os.Exit(1)
	}

	// check if it needs to login automatically
	if exists(encryptedFile) {
		if err := p.loginByFile(); err != nil {
			log.Errorf("login by encrypt file: %v", err)
			os.Exit(1)
		}
	}

	// context for controlling the goroutines
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	// start the http server
	p.startServer(&wg)

	wg.Add(1)
	// start the heartbeat goroutine
	go startCronJob(ctx, &wg, p.settings.App.Heartbeat, p.heartbeat)

	wg.Add(1)
	// start the collect goroutine
	go startCronJob(ctx, &wg, p.settings.App.Collect, p.collect)

	// block until receive a exit signal
	for {
		select {
		case <-p.exit:
			log.Info("receive the exit signal")

			// flush the log
			log.Sync()

			start := time.Now()

			// stop the http server
			p.httpServer.Shutdown(context.Background())

			// stop the cron job
			cancel()

			// wait for all the goroutines exit
			wg.Wait()

			// close the lock file
			windows.CloseHandle(handle)

			log.Infof("shut down takes time: %v", time.Now().Sub(start))
			return nil
		}
	}
}

// Stop the service
func (p *Program) Stop(s service.Service) error {
	// signal the program to exit
	close(p.exit)

	// close the grpc connection
	if p.conn != nil {
		p.conn.Close()
	}

	return nil
}

func (p *Program) hasLockfile() (windows.Handle, error) {
	if !exists(p.lockFile) {
		// create the lock file if it doesn't exist
		f, err := os.Create(p.lockFile)
		if err != nil {
			return windows.InvalidHandle, err
		}
		f.Close()
	}
	filePath, err := syscall.UTF16PtrFromString(p.lockFile)
	if err != nil {
		return windows.InvalidHandle, err
	}
	// try to read this file - no sharing
	return windows.CreateFile(filePath, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
}
