package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

var debug bool
var cmd string

func init() {
	flag.BoolVar(&debug, "debug", false, "Enable debug logging. Default : 'false'")
	flag.StringVar(&cmd, "c", "", "Run command inside the container, using login shell")
	flag.Parse()
}

func main() {
	logrus.Debug("Starting dockersh")

	logrus.Debug("Loading all config files")
	config, err := loadAllConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not load config: %v\n", err)
		return
	}

	if config.LogFile != "" {
		logFile, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			logrus.Debugf("Failed to open log file %s for output: %s", config.LogFile, err)
		} else {
			logrus.SetOutput(logFile)
			logrus.RegisterExitHandler(func() {
				if logFile == nil {
					return
				}
				logFile.Close()
			})
		}
	}

	if config.LogLevel != "" {
		ll, err := logrus.ParseLevel(config.LogLevel)
		if err == nil {
			logrus.SetLevel(ll)
		} else {
			logrus.SetLevel(logrus.InfoLevel)
		}
	}

	logrus.Debugf("Config dump: %+v", config)

	logrus.Debugf("Checking for container: name=%v", config.ContainerName)
	id, err := isContainerRunning(config.ContainerName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not check container status: %v\n", err)
		return
	}
	logrus.Debugf("Container running? %v", id != "")

	if id == "" {
		logrus.Debug("Container is not running, starting it")
		id, err = startContainer(config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not start container: %s\n", err)
			logrus.Debugf("could not start container: %s\n", err)
			return
		}
	}

	logrus.Debugf("Container ID: %v", id)
	logrus.Debug("Exec into the container")

	err = execContainer2(id, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not exec into container: %v\n", err)
	}

	return
}
