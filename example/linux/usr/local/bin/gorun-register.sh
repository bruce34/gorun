#!/bin/bash

if [ -e /proc/sys/fs/binfmt_misc/golang ]; then
  echo -1 > /proc/sys/fs/binfmt_misc/golang
fi
echo ':golang:E::go::/usr/local/bin/gorun:OC' > /proc/sys/fs/binfmt_misc/register

if [ -e /proc/sys/fs/binfmt_misc/golangcomment ]; then
  echo -1 > /proc/sys/fs/binfmt_misc/golangcomment
fi
echo ':golangcomment:M:0:///bin/env gorun::/usr/local/bin/gorun:OC' > /proc/sys/fs/binfmt_misc/register
