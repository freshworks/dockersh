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

	lvl, ok := os.LookupEnv("LOG_LEVEL")
	if ok {
		ll, err := logrus.ParseLevel(lvl)
		if err == nil {
			logrus.SetLevel(ll)
		}
	} else {
		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		} else {
			logrus.SetLevel(logrus.InfoLevel)
		}
	}
}

func main() {
	logrus.Debug("Starting dockersh")

	logrus.Debug("Loading all config files")
	config, err := loadAllConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not load config: %v\n", err)
		return
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
			return
		}
	}

	logrus.Debugf("Container ID: %v", id)
	logrus.Debug("Exec into the container")

	err = execContainer(id, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not exec into container: %v\n", err)
	}

	return
}
