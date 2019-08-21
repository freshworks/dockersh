package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"gopkg.in/gcfg.v1"
)

type Configuration struct {
	ImageName                   string
	EnableUserImageName         bool
	ContainerName               string
	EnableUserContainerName     bool
	MountHomeFrom               string
	EnableUserMountHomeFrom     bool
	MountHomeTo                 string
	EnableUserMountHomeTo       bool
	UserCwd                     string
	EnableUserUserCwd           bool
	ContainerUsername           string
	EnableUserContainerUsername bool
	Shell                       string
	EnableUserShell             bool
	EnableUserConfig            bool
	MountHome                   bool
	EnableUserMountHome         bool
	MountTmp                    bool
	EnableUserMountTmp          bool
	MountDockerSocket           bool
	EnableUserMountDockerSocket bool
	DockerSocket                string
	EnableUserDockerSocket      bool
	Entrypoint                  string
	EnableUserEntrypoint        bool
	Cmd                         []string
	EnableUserCmd               bool
	Env                         []string
	EnableUserEnv               bool
	ReverseForward              []string
	EnableUserReverseForward    bool
	UserId                      int
	GroupId                     int
}

func (c Configuration) Dump() string {
	return fmt.Sprintf("ImageName %s MountHomeTo %s ContainerUsername %s Shell %s DockerSocket %s", c.ImageName, c.MountHomeTo, c.ContainerUsername, c.Shell, c.DockerSocket)
}

type configInterpolation struct {
	Home string
	User string
}

var defaultConfig = Configuration{
	ImageName:         "busybox",
	ContainerName:     "%u_dockersh",
	MountHomeFrom:     "%h",
	MountHomeTo:       "%h",
	UserCwd:           "%h",
	ContainerUsername: "%u",
	Shell:             "/bin/ash",
	DockerSocket:      "/var/run/docker.sock",
	Entrypoint:        "internal",
}

func loadAllConfig() (config Configuration, err error) {
	username, homedir, uid, gid, err := getCurrentUser()
	if err != nil {
		return config, err
	}

	config, err = loadConfig(loadableFile("/etc/dockersh"), username)
	if err != nil {
		return config, err
	}

	if config.EnableUserConfig == true {
		userconfig, err := loadConfig(loadableFile(fmt.Sprintf("%s/.dockersh", homedir)), username)
		if err != nil {
			return config, err
		}
		config = mergeConfigs(mergeConfigs(defaultConfig, config, false), userconfig, true)
	} else {
		config = mergeConfigs(defaultConfig, config, false)
	}

	configInterpolations := configInterpolation{homedir, username}
	err = getInterpolatedConfig(&config, configInterpolations)
	if err == nil {
		config.ContainerName = config.ContainerName + "_" + strings.Replace(config.ImageName, ":", "_", -1)
	}

	config.UserId = uid
	config.GroupId = gid

	return config, err
}

type loadableFile string

func (fn loadableFile) Getcontents() ([]byte, error) {
	localConfigFile, err := os.Open(string(fn))
	var b []byte
	if err != nil {
		return b, fmt.Errorf("Could not open: %s", string(fn))
	}
	b, err = ioutil.ReadAll(localConfigFile)
	if err != nil {
		return b, err
	}
	localConfigFile.Close()
	return b, nil
}

func loadConfig(filename loadableFile, user string) (config Configuration, err error) {
	bytes, err := filename.Getcontents()
	if err != nil {
		return config, err
	}
	return loadConfigFromString(bytes, user)
}

func mergeConfigs(old Configuration, new Configuration, blacklist bool) (ret Configuration) {
	if (!blacklist || old.EnableUserShell) && new.Shell != "" {
		old.Shell = new.Shell
	}
	if (!blacklist || old.EnableUserContainerUsername) && new.ContainerUsername != "" {
		old.ContainerUsername = new.ContainerUsername
	}
	if (!blacklist || old.EnableUserImageName) && new.ImageName != "" {
		old.ImageName = new.ImageName
	}
	if (!blacklist || old.EnableUserMountHomeTo) && new.MountHomeTo != "" {
		old.MountHomeTo = new.MountHomeTo
	}
	if (!blacklist || old.EnableUserMountHomeFrom) && new.MountHomeFrom != "" {
		old.MountHomeFrom = new.MountHomeFrom
	}
	if (!blacklist || old.EnableUserDockerSocket) && new.DockerSocket != "" {
		old.DockerSocket = new.DockerSocket
	}
	if (!blacklist || old.EnableUserMountHome) && new.MountHome == true {
		old.MountHome = true
	}
	if (!blacklist || old.EnableUserMountTmp) && new.MountTmp == true {
		old.MountTmp = true
	}
	if (!blacklist || old.EnableUserMountDockerSocket) && new.MountDockerSocket == true {
		old.MountDockerSocket = true
	}
	if (!blacklist || old.EnableUserEntrypoint) && new.Entrypoint != "" {
		old.Entrypoint = new.Entrypoint
	}
	if (!blacklist || old.EnableUserUserCwd) && new.UserCwd != "" {
		old.UserCwd = new.UserCwd
	}
	if (!blacklist || old.EnableUserContainerName) && new.ContainerName != "" {
		old.ContainerName = new.ContainerName
	}
	if (!blacklist || old.EnableUserCmd) && len(new.Cmd) > 0 {
		old.Cmd = new.Cmd
	}
	if (!blacklist || old.EnableUserEnv) && len(new.Env) > 0 {
		old.Env = new.Env
	}
	if (!blacklist || old.EnableUserReverseForward) && len(new.ReverseForward) > 0 {
		old.ReverseForward = new.ReverseForward
	}
	if !blacklist && new.EnableUserConfig == true {
		old.EnableUserConfig = true
	}
	return old
}

func loadConfigFromString(bytes []byte, user string) (config Configuration, err error) {
	inicfg := struct {
		Dockersh Configuration
		User     map[string]*Configuration
	}{}
	err = gcfg.ReadStringInto(&inicfg, string(bytes))
	if err != nil {
		return config, err
	}
	if inicfg.User[user] == nil {
		return inicfg.Dockersh, nil
	}
	return mergeConfigs(inicfg.Dockersh, *inicfg.User[user], false), nil
}

func tmplConfigVar(template string, v *configInterpolation) string {
	shell := "/bin/bash"
	r := strings.NewReplacer("%h", v.Home, "%u", v.User, "%s", shell) // Arguments are old, new ...
	return r.Replace(template)
}

func getInterpolatedConfig(config *Configuration, configInterpolations configInterpolation) error {
	config.ContainerUsername = tmplConfigVar(config.ContainerUsername, &configInterpolations)
	config.MountHomeTo = tmplConfigVar(config.MountHomeTo, &configInterpolations)
	config.MountHomeFrom = tmplConfigVar(config.MountHomeFrom, &configInterpolations)
	config.ImageName = tmplConfigVar(config.ImageName, &configInterpolations)
	config.Shell = tmplConfigVar(config.Shell, &configInterpolations)
	config.UserCwd = tmplConfigVar(config.UserCwd, &configInterpolations)
	config.ContainerName = tmplConfigVar(config.ContainerName, &configInterpolations)

	for i, e := range config.Env {
		e = os.ExpandEnv(e)
		config.Env[i] = tmplConfigVar(e, &configInterpolations)
	}

	return nil
}
