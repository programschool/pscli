package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

type Docker struct {
	cli *client.Client
	ctx context.Context
}

const runSH = "/programschool/server/run.sh"

var baseDir, imageName string

func main() {
	client := Docker{}.client()

	version := "1.0.4"
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(fmt.Sprintf("version: %s", version))
		os.Exit(0)
	}

	if len(os.Args) != 3 {
		fmt.Println(fmt.Sprintf("version: %s", version))
		fmt.Println("构建镜像：\n\t1. 参数1 指定根目录\n\t2. 参数2 指定目标镜像名称（格式：myname/name）\n\t示例：pscli . myname/myimage")
		fmt.Println("测试镜像：\n\tdocker run --rm --network host -it boxlayer.com/myname/name")
		os.Exit(0)
	}

	baseDir = os.Args[1]
	imageName = os.Args[2]

	_, err := client.buildImage("./Dockerfile", baseDir, imageName)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\033[32m")
	fmt.Println(fmt.Sprintf("\nBuild image %s", imageName))
	fmt.Println("\033[0m")
	client.reBuildImage(imageName)
}

func (docker Docker) client() Docker {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Println(err.Error())
	}

	docker.cli = cli
	docker.ctx = context.Background()
	defer docker.cli.Close()

	return docker
}

func createTar(srcDir, tarFIle string) error {
	/* #nosec */
	c := exec.Command("tar", "-cf", tarFIle, "-C", srcDir, ".")
	if err := c.Run(); err != nil {
		return nil
	}
	return nil
}

func tempFileName(prefix, suffix string) (string, error) {
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}

	return filepath.Join(os.TempDir(), prefix+hex.EncodeToString(randBytes)+suffix), nil
}

func (docker Docker) reBuildImage(imageName string) {
	inspect, _, _ := docker.cli.ImageInspectWithRaw(docker.ctx, imageName)

	nweDockerfile := [][]string{
		{"FROM ", imageName, "\n\n"},

		{"RUN mkdir -p /programschool/server"},
		{"WORKDIR \"/\""},

		{"RUN curl -sSL https://build.boxlayer.com | bash\n"},
	}

	var workDir string

	if len(inspect.Config.WorkingDir) > 0 {
		workDir = inspect.Config.WorkingDir
		nweDockerfile = append(nweDockerfile, []string{fmt.Sprintf("RUN chown -R ubuntu.root %s\n", workDir)})
	} else {
		nweDockerfile = append(nweDockerfile, []string{"RUN mkdir -p /home/ubuntu/learn"})
		nweDockerfile = append(nweDockerfile, []string{"RUN chown -R ubuntu.root /home/ubuntu/learn"})
		workDir = "/home/ubuntu/learn"
	}
	if len(inspect.Config.Cmd) > 0 {
		cmd := []byte(strings.Join(inspect.Config.Cmd, " "))
		nweDockerfile = append(nweDockerfile, []string{
			"RUN echo '",
			fmt.Sprintf("#!/bin/bash\\n\\nnohup %s &\\n", cmd),
			"CURRENT_DIR=$(pwd)/$(dirname $0)\\n",
			fmt.Sprintf("su -l ubuntu -c \"${CURRENT_DIR}/code-server/bin/code-server --disable-update-check --auth=none --bind-addr 0.0.0.0:2090 %s\"", workDir),
			"'",
			fmt.Sprintf(" > %s", runSH),
		})
	} else {
		nweDockerfile = append(nweDockerfile, []string{
			"RUN echo '",
			fmt.Sprintf("#!/bin/bash\\n"),
			"CURRENT_DIR=$(pwd)/$(dirname $0)\\n",
			fmt.Sprintf("su -l ubuntu -c \"${CURRENT_DIR}/code-server/bin/code-server --disable-update-check --auth=none --bind-addr 0.0.0.0:2090 %s\"", workDir),
			"'",
			fmt.Sprintf(" > %s", runSH),
		})
	}

	workDirFile := fmt.Sprintf("%s/WORKDIR", baseDir)
	if _, err := os.Stat(workDirFile); err == nil {
		// path/to/whatever exists
		nweDockerfile = append(nweDockerfile, []string{
			fmt.Sprintf("COPY WORKDIR %s", workDir),
		})
	}

	if len(inspect.Config.Env) > 0 {
		var environment []string
		for i := range inspect.Config.Env {
			environment = append(environment, fmt.Sprintf("%s\\n", inspect.Config.Env[i]))
		}
		nweDockerfile = append(nweDockerfile, []string{
			fmt.Sprintf("RUN echo '%s' >> /etc/environment", strings.Join(environment, "")),
		})
	}

	nweDockerfile = append(nweDockerfile, []string{fmt.Sprintf("CMD [\"bash\", \"%s\"]", runSH)})

	temp := fmt.Sprintf("%s/Dockerfile.temp", baseDir)
	if err := os.Remove("Dockerfile.temp"); err != nil {
		// no such file Dockerfile.temp
	}
	dockerfileTemp, err := os.OpenFile(temp, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer dockerfileTemp.Close()

	for i := range nweDockerfile {
		if _, err = dockerfileTemp.WriteString(strings.Join(nweDockerfile[i], "")); err != nil {
			panic(err)
		}
		if _, err = dockerfileTemp.WriteString("\n"); err != nil {
			panic(err)
		}
	}

	fmt.Println("\n...\n")

	fullName := fmt.Sprintf("boxlayer.com/%s", imageName)
	_, err = docker.buildImage("Dockerfile.temp", baseDir, fullName)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.Remove(temp); err != nil {
		// no such file Dockerfile.temp
	}

	if checkErr := docker.checkImage(fullName); checkErr != nil {
		fmt.Println("\033[31m")
		fmt.Println("Build Error: ")
		fmt.Println("镜像构建失败，请检查构建步骤，或者参考文档 https://www.programschool.com/docs/build-image")
		fmt.Println("\033[0m")
	} else {
		fmt.Println("\033[32m")
		fmt.Println(fmt.Sprintf("Build image %s\n", fullName))
		fmt.Println(fmt.Sprintf("Build image success ✨✨！"))
		fmt.Println(fmt.Sprintf("Next，test image and push image\n"))
		fmt.Println("\033[33m")
		fmt.Println(fmt.Sprintf("Test：\ndocker run --rm --network host -it %s\n", fullName))
		fmt.Println(fmt.Sprintf("Push：\ndocker login boxlayer.com"))
		fmt.Println(fmt.Sprintf("docker push %s\n", fullName))
		fmt.Println("\033[0m")
	}
}

func (docker Docker) checkImage(fullName string) error {
	resp, err := docker.cli.ContainerCreate(
		docker.ctx,
		&container.Config{
			Image:        fullName,
			Tty:          false,
			User:         "root",
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			OpenStdin:    true,
		}, nil, nil, nil, "")

	if err != nil {
		panic(err)
	}

	testErr := docker.cli.ContainerStart(docker.ctx, resp.ID, types.ContainerStartOptions{})

	exec, _ := docker.cli.ContainerExecCreate(docker.ctx, resp.ID, types.ExecConfig{
		User:         "root",
		Privileged:   false,
		Tty:          true,
		AttachStdin:  true,
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          []string{"bash", fmt.Sprintf("%s", runSH)},
	})

	execErr := docker.cli.ContainerExecStart(docker.ctx, exec.ID, types.ExecStartCheck{
		Detach: true,
		Tty:    false,
	})

	timeout := 0 * time.Second
	docker.cli.ContainerStop(docker.ctx, resp.ID, &timeout)
	docker.cli.ContainerRemove(docker.ctx, resp.ID, types.ContainerRemoveOptions{})

	if execErr != nil {
		docker.cli.ImageRemove(docker.ctx, fullName, types.ImageRemoveOptions{})
		return execErr
	}

	if testErr != nil {
		docker.cli.ImageRemove(docker.ctx, fullName, types.ImageRemoveOptions{})
		return testErr
	}
	return nil
}

func (docker Docker) buildImage(dockerfile, baseDir, name string) ([]string, error) {
	authConfig := types.AuthConfig{
		Username: "image",
		Password: "Z29kYWRkeQ==",
	}
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		panic(err)
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)

	reader, err := docker.cli.ImagePull(docker.ctx, "boxlayer.com/official/base", types.ImagePullOptions{
		RegistryAuth: authStr,
	})
	if err != nil {
		panic(err)
	}
	io.Copy(os.Stdout, reader)

	tarFile, err := tempFileName("docker-", ".image")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tarFile)

	if err := createTar(baseDir, tarFile); err != nil {
		return nil, err
	}

	/* #nosec */
	dockerFileTarReader, err := os.Open(tarFile)
	if err != nil {
		return nil, err
	}
	defer dockerFileTarReader.Close()

	ctx, cancel := context.WithTimeout(docker.ctx, time.Duration(300)*time.Second)
	defer cancel()

	buildArgs := make(map[string]*string)

	PWD, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	defer os.Chdir(PWD)

	if err := os.Chdir(baseDir); err != nil {
		return nil, err
	}

	resp, err := docker.cli.ImageBuild(
		ctx,
		dockerFileTarReader,
		types.ImageBuildOptions{
			Dockerfile: dockerfile,
			Tags:       []string{name},
			NoCache:    true,
			Remove:     true,
			BuildArgs:  buildArgs,
		})

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var messages []string

	rd := bufio.NewReader(resp.Body)
	for {
		n, _, err := rd.ReadLine()
		if err != nil && err == io.EOF {
			break
		} else if err != nil {
			return messages, err
		}

		var step map[string]interface{}

		if err = json.Unmarshal(n, &step); err != nil {
			log.Fatal(err)
		}

		fmt.Println(step["stream"])
		messages = append(messages, string(n))
	}

	return messages, nil
}
