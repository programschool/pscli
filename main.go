package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

func main() {
	if len(os.Args) != 3 {
		fmt.Println("bad num of arguments:\n\t1. = baseDir with image content\n\t2. = image name")
		os.Exit(0)
	}

	client := Docker{}.client()
	baseDir := os.Args[1]
	imageName := os.Args[2]
	_, err := client.buildImage("./Dockerfile", baseDir, imageName)
	if err != nil {
		log.Fatal(err)
	}

	//cli := Docker{}.client().cli

	fmt.Println(fmt.Sprintf("\nBuild image %s", imageName))

	client.rebuildImage(imageName)
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

func (docker Docker) rebuildImage(imageName string) {
	inspect, _, _ := docker.cli.ImageInspectWithRaw(docker.ctx, imageName)

	nweDockerfile := [][]string{
		{"FROM ", imageName, "\n\n"},
		{"# RUN curl -sSL https://build.boxlayer.com | bash\n"},
	}

	if len(inspect.Config.Env) > 0 {
		for i := range inspect.Config.Env {
			nweDockerfile = append(nweDockerfile, []string{"ENV ", strings.Replace(inspect.Config.Env[i], "=", " ", 1)})
		}
	}
	if len(inspect.Config.Entrypoint) > 0 {
		entrypoint := []byte(strings.Join(inspect.Config.Entrypoint, "\", \""))
		nweDockerfile = append(nweDockerfile, []string{"ENTRYPOINT [\"", string(entrypoint), "\"]"})
	}
	if len(inspect.Config.Cmd) > 0 {
		cmd := []byte(strings.Join(inspect.Config.Cmd, "\", \""))
		nweDockerfile = append(nweDockerfile, []string{"CMD [\"", string(cmd), "\"]"})
	}

	if err := os.Remove("Dockerfile.temp"); err != nil {
		// no such file Dockerfile.temp
	}
	dockerfileTemp, err := os.OpenFile("Dockerfile.temp", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
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

	fullName := fmt.Sprintf("boxlayer.com/%s", os.Args[2])
	_, err = docker.buildImage("./Dockerfile.temp", os.Args[1], fullName)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.Remove("Dockerfile.temp"); err != nil {
		// no such file Dockerfile.temp
	}

	fmt.Println(fmt.Sprintf("Build image %s\n", fullName))
	fmt.Println(fmt.Sprintf("Build image success ✨✨！"))
	fmt.Println(fmt.Sprintf("Next，test image and push image\n"))
	fmt.Println(fmt.Sprintf("Test：\npscli --test %s\n", fullName))
	fmt.Println(fmt.Sprintf("Push：\ndocker login boxlayer.com"))
	fmt.Println(fmt.Sprintf("docker push %s\n", fullName))
}

func (docker Docker) buildImage(dockerfile, baseDir, name string) ([]string, error) {

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

		//fmt.Println(step["stream"])
		messages = append(messages, string(n))
	}

	return messages, nil
}
