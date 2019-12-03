package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/context"
)

func isContainerRunning(name string) (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", err
	}

	filter := filters.NewArgs()
	filter.Add("name", name)
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{Filters: filter})
	if err != nil {
		return "", err
	}

	if len(containers) >= 1 {
		return containers[0].ID, nil
	}

	return "", nil
}

func containerID(name string) (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", err
	}

	filter := filters.NewArgs()
	filter.Add("name", name)
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true, Filters: filter})
	if err != nil {
		return "", err
	}

	if len(containers) >= 1 {
		return containers[0].ID, nil
	}

	return "", nil
}

func startContainer(config Configuration) (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", err
	}

	id, err := containerID(config.ContainerName)
	logrus.Debugf("Checking if container with name %v already exists: %v", config.ContainerName, id != "")

	if id != "" {
		logrus.Debugf("Removing container, name: %v id: %v", config.ContainerName, id)
		err := cli.ContainerRemove(context.Background(), id,
			types.ContainerRemoveOptions{})
		if err != nil {
			return "", err
		}
	}

	binds := []string{"/etc/passwd:/etc/passwd:ro", "/etc/group:/etc/group:ro"}

	var init []string
	if config.Entrypoint != "" {
		init = []string{config.Entrypoint}
	}
	logrus.Debugf("Entry point is: %v", init)

	var env []string
	for _, e := range config.Env {
		env = append(env, e)
	}

	if config.MountTmp {
		logrus.Debugf("Bind mounting /tmp")
		binds = append(binds, "/tmp:/tmp:rw")
	}
	if config.MountHome {
		h := fmt.Sprintf("%s:%s:rw", config.MountHomeFrom, config.MountHomeTo)
		logrus.Debugf("Bind mounting home: %v", h)
		binds = append(binds, h)
	}
	if config.MountDockerSocket {
		logrus.Debugf("Bind mounting %v", config.DockerSocket)
		binds = append(binds, config.DockerSocket+":/var/run/docker.sock")
	}

	hostname, _ := os.Hostname()

	ctx := context.Background()

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Hostname:        hostname,
			User:            fmt.Sprintf("%d:%d", config.UserId, config.GroupId),
			AttachStdin:     false,
			AttachStdout:    false,
			AttachStderr:    false,
			Tty:             false,
			OpenStdin:       false,
			StdinOnce:       false,
			Env:             env,
			Healthcheck:     nil,
			Image:           config.ImageName,
			Volumes:         nil,
			WorkingDir:      config.UserCwd,
			Entrypoint:      init,
			NetworkDisabled: false,
			Labels:          map[string]string{"user": config.ContainerUsername},
			StopSignal:      "",
			StopTimeout:     nil,
			Shell:           []string{"/bin/bash"},
		},
		&container.HostConfig{
			Binds:      binds,
			AutoRemove: true,
			// Applicable to UNIX platforms
			CapAdd:          nil,
			CapDrop:         []string{"SETUID", "SETGID", "NET_RAW", "MKNOD"},
			Capabilities:    nil,
			Privileged:      false,
			PublishAllPorts: false,
			ReadonlyRootfs:  true,
			SecurityOpt:     nil, // TODO: Enable selinux etc
			Resources: container.Resources{
				Memory: config.MemoryQuota,
			},
			Mounts: []mount.Mount{
				{
					Target:   "/var/run/screen",
					Source:   "",
					Type:     "volume",
					ReadOnly: false,
				},
			},
			//UsernsMode:      UsernsMode, // TODO: Enable the user namespace to use for the container
		},
		nil, config.ContainerName)
	if err != nil {
		return "", err
	}

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", err
	}

	return resp.ID, nil
}

func execContainer(id string, config Configuration) error {
	// TODO: Move to proper docker client API

	dockerBinary, err := exec.LookPath("docker")
	if err != nil {
		return err
	}

	args := []string{dockerBinary}
	args = append(args, "exec")
	args = append(args, "--user")
	args = append(args, fmt.Sprintf("%d:%d", config.UserId, config.GroupId))
	args = append(args, "--workdir")
	args = append(args, config.UserCwd)

	for _, e := range config.Env {
		args = append(args, "-e")
		args = append(args, e)
	}

	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		args = append(args, "--tty")
	}

	args = append(args, "--interactive")

	args = append(args, id)

	args = append(args, config.Shell)
	if cmd != "" {
		args = append(args, "-c")
		args = append(args, cmd)
	} else {
		args = append(args, "--login")
		if os.Getenv("PS1") != "" {
			args = append(args, "-i")
		}
	}

	if err := syscall.Exec(args[0], args, os.Environ()); err != nil {
		return err
	}

	return nil
}

func dockerExecCleanup(config Configuration, dockerId string, dockerBinary string, pidfile string) {
	a := []string{"exec"}
	a = append(a, dockerId)
	a = append(a, config.Shell)
	a = append(a, "-c")
	a = append(a, fmt.Sprintf("if [ -f %v ]; then kill -HUP -$(cat %v); rm %v; fi", pidfile, pidfile, pidfile))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c := exec.CommandContext(ctx, dockerBinary, a...)

	logrus.Debugf("Docker exec cleanup, running: %v", c)
	err := c.Run()
	if err == nil {
		logrus.Debugf("docker exec cleanup finished")
		return
	}

	logrus.Debugf("Error docker exec cleanup: %v", err)

	if ctx.Err() == context.DeadlineExceeded {
		logrus.Debugf("docker exec cleanup timedout")
	}
}

func execContainer2(id string, config Configuration) error {
	dockerBinary, err := exec.LookPath("docker")
	if err != nil {
		return err
	}

	args := []string{"exec"}
	args = append(args, "--user")
	args = append(args, fmt.Sprintf("%d:%d", config.UserId, config.GroupId))
	args = append(args, "--workdir")
	args = append(args, config.UserCwd)

	for _, e := range config.Env {
		args = append(args, "-e")
		args = append(args, e)
	}

	if terminal.IsTerminal(int(os.Stdout.Fd())) {
		args = append(args, "--tty")
	}

	args = append(args, "--interactive")
	args = append(args, id)
	args = append(args, config.Shell)

	pidfile := fmt.Sprintf("/tmp/dockersh-exec-%d", os.Getpid())

	if cmd != "" {
		args = append(args, "-c")
		args = append(args, cmd)
	} else {
		args = append(args, "--login")
		if os.Getenv("PS1") != "" {
			args = append(args, "-i")
		}

		args = append(args, "-c")
		args = append(args, fmt.Sprintf("echo $$ > %s; exec %v", pidfile, config.Shell))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := exec.CommandContext(ctx, dockerBinary, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = os.Environ()

	// When this "dockersh" process is signalled/killed (like when ssh
	// disconnects due to network issue etc), due to a docker bug,
	// processes that are running inside 'docker exec' are left as they
	// are i.e., they will continue to run. This can add up quite a bit
	// of processes over the time. See here for more information:
	// https://github.com/moby/moby/issues/9098
	//
	// Until that is fixed, we will handle it here by:
	//
	//  - Have the pid of exec'ed bash at /tmp/dockersh-exec-${this_process_pid}
	//
	//  - Install a signal handler here that will send SIGHUP to the pid
	//    stored in /tmp/dockersh-exec-${this_process_pid}
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	go func() {
		s := <-sigc
		logrus.Debugf("Handling signal: %v", s)
		cancel()
	}()

	defer dockerExecCleanup(config, id, dockerBinary, pidfile)

	logrus.Debugf("Running command and waiting for it to finish: %v", c)
	err = c.Run()
	logrus.Debugf("Command finished: %v", err)

	return err
}
