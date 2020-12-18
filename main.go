package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
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
		fmt.Println("bad num of arguments:\n\t1. = dir with image content\n\t2. = image name")
		os.Exit(0)
	}

	client := Docker{}.client()
	msg, err := client.buildImage(os.Args[1], os.Args[2])
	if err != nil {
		log.Fatal(err)
	}

	//cli := Docker{}.client().cli

	fmt.Println(msg)

	fmt.Println("\n\n")

	client.rebuildImage(os.Args[2])
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

func (docker Docker) rebuildImage(name string) {
	s, _, _ := docker.cli.ImageInspectWithRaw(docker.ctx, name)
	fmt.Println(s.Config.Cmd)
	fmt.Println(s.Config.Entrypoint)
	fmt.Println(s.Config.Env)

	env := []byte(strings.Join(s.Config.Env, "\", \""))
	envstr := []string{"ENV [\"", string(env), "\"]"}

	f, err := os.OpenFile("dat1", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	//if _, err = f.WriteString("text"); err != nil {
	//	panic(err)
	//}

	f.WriteString(strings.Join(envstr, ""))
	f.WriteString("\n\n")
	f.WriteString(strings.Join(envstr, ""))

}

func (docker Docker) buildImage(dir, name string) ([]string, error) {

	tarFile, err := tempFileName("docker-", ".image")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tarFile)

	if err := createTar(dir, tarFile); err != nil {
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

	if err := os.Chdir(dir); err != nil {
		return nil, err
	}

	resp, err := docker.cli.ImageBuild(
		ctx,
		dockerFileTarReader,
		types.ImageBuildOptions{
			Dockerfile: "./Dockerfile",
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
		messages = append(messages, string(n))
	}

	return messages, nil
}
