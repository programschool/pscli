#!/bin/bash

export GOOS=linux
export GOARCH=amd64

go fmt main.go
go build -o pscli main.go
cd ..
