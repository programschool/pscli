#!/bin/bash

export GOOS=linux
export GOARCH=amd64

go fmt main.go
go build -o ../../container-node/pscli main.go
cd ..
