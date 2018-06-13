# ahs

[![GoDoc](https://godoc.org/github.com/mvisonneau/ahs?status.svg)](https://godoc.org/github.com/mvisonneau/ahs)
[![Go Report Card](https://goreportcard.com/badge/github.com/mvisonneau/ahs)](https://goreportcard.com/report/github.com/mvisonneau/ahs)
[![Docker Pulls](https://img.shields.io/docker/pulls/mvisonneau/ahs.svg)](https://hub.docker.com/r/mvisonneau/ahs/)
[![Build Status](https://travis-ci.org/mvisonneau/ahs.svg?branch=master)](https://travis-ci.org/mvisonneau/ahs)
[![Coverage Status](https://coveralls.io/repos/github/mvisonneau/ahs/badge.svg?branch=master)](https://coveralls.io/github/mvisonneau/ahs?branch=master)

This projects aims to ease the configuration of AWS EC2 instances hostname.
In particular when they are launched as part of ASGs or fleets.

## TL;DR

```
~$ wget https://github.com/mvisonneau/ahs/releases/download/0.0.1/ahs_linux_amd64 -O /usr/local/bin/ahs; chmod +x /usr/local/bin/ahs
~$ ahs run
INFO[2018-06-13T21:58:12Z] Found instance-id : 'i-07263d49fca824ba5'
INFO[2018-06-13T21:58:12Z] Found AZ: 'eu-west-1a'
INFO[2018-06-13T21:58:12Z] Computed region : 'eu-west-1'
INFO[2018-06-13T21:58:12Z] Found instance name tag : 'myhostname'
INFO[2018-06-13T21:58:12Z] Computed unique hostname : 'myhostname-07263'
INFO[2018-06-13T21:58:12Z] Setting instance hostname locally
INFO[2018-06-13T21:58:12Z] Setting hostname on configured instance tag 'Name'
```

## Usage

```
~$ ahs
NAME:
   ahs - Set the hostname of an EC2 instance based on a tag value and the instance-id

USAGE:
   ahs [global options] command [command options] [arguments...]

COMMANDS:
     run      replace the hostname with found/computed values
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --dry-run           only display what would have been done [$AHS_DRY_RUN]
   --id-length value   length of the id to keep in the hostname (default: 5) [$AHS_ID_LENGTH]
   --input-tag value   tag to use as input to determine the hostname (default: "Name") [$AHS_TAG_NAME_INPUT]
   --log-level value   log level (debug,info,warn,fatal,panic) (default: "info") [$AHS_LOG_LEVEL]
   --log-format value  log format (json,text) (default: "text") [$AHS_LOG_FORMAT]
   --output-tag value  tag to update with the computed hostname (default: "Name") [$AHS_TAG_NAME_OUTPUT]
   --separator value   separator to use between tag and id (default: "-") [$AHS_SEPARATOR]
   --help, -h          show help
   --version, -v       print the version
```
