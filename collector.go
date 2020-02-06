package main

import (
	"TL-Data-Agent/config"
	"TL-Data-Agent/log"
	"flag"

	"github.com/kardianos/service"
)

var (
	action = flag.String("action", "", "Service action for collector: start, stop, restart, install, uninstall")
)

// updateOptions updates the log options
func updateOptions(scope string, options *log.Options, settings *config.Config) error {
	if settings.Log.OutputPath != "" {
		options.OutputPaths = []string{settings.Log.OutputPath}
	}
	if settings.Log.RotationPath != "" {
		options.RotateOutputPath = settings.Log.RotationPath
	}
	options.RotationMaxBackups = settings.Log.RotationMaxBackups
	options.RotationMaxSize = settings.Log.RotationMaxSize
	options.RotationMaxAge = settings.Log.RotationMaxAge
	options.JSONEncoding = settings.Log.JSONEncoding
	level, err := options.ConvertLevel(settings.Log.OutputLevel)
	if err != nil {
		return err
	}
	options.SetOutputLevel(scope, level)
	options.SetLogCallers(scope, true)

	return nil
}

func main() {
	flag.Parse()

	// init the settings
	var settings config.Config
	// parse the config file
	if err := config.ParseYamlFile("config.yml", &settings); err != nil {
		panic(err)
	}

	// init and update the log options
	logOptions := log.DefaultOptions()
	if err := updateOptions("default", logOptions, &settings); err != nil {
		panic(err)
	}
	// configure the log options
	if err := log.Configure(logOptions); err != nil {
		panic(err)
	}

	// define the service config for program
	options := make(service.KeyValue)
	options["Restart"] = "on-success"
	options["SuccessExitStatus"] = "1 2 8 SIGKILL"
	serviceConfig := &service.Config{
		Name:         "TraderLinkedDataCollector",
		DisplayName:  "Trader Linked Data Collector Service",
		Description:  "The service that collects information from trading platforms",
		Dependencies: []string{},
		Option:       options,
	}

	prog := NewProgram(&settings)
	// new the service for program
	s, err := service.New(prog, serviceConfig)
	if err != nil {
		panic(err)
	}

	// if it's the service action
	if len(*action) > 0 {
		if err := service.Control(s, *action); err != nil {
			panic(err)
		}
		return
	}

	// start the application
	if err := s.Run(); err != nil {
		log.Errorf("start the application: %v", err)
	}
}
