#!/bin/sh -x

go install -ldflags "-X main.Version=`git describe --always --dirty`" github.com/kkrs/gossh/cmd/gossh
