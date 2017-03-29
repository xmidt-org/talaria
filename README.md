# talaria

[![Build Status](https://travis-ci.org/Comcast/talaria.svg?branch=master)](https://travis-ci.org/Comcast/talaria) 
[![codecov.io](http://codecov.io/github/Comcast/talaria/coverage.svg?branch=master)](http://codecov.io/github/Comcast/talaria?branch=master)
[![Code Climate](https://codeclimate.com/github/Comcast/talaria/badges/gpa.svg)](https://codeclimate.com/github/Comcast/talaria)
[![Issue Count](https://codeclimate.com/github/Comcast/talaria/badges/issue_count.svg)](https://codeclimate.com/github/Comcast/talaria)
[![Go Report Card](https://goreportcard.com/badge/github.com/Comcast/talaria)](https://goreportcard.com/report/github.com/Comcast/talaria)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/Comcast/talaria/blob/master/LICENSE)

The Webpa routing agent.

# How to Install

## Centos 6

1. Import the public GPG key (replace `v0.0.1-65alpha` with the release you want)

```
rpm --import https://github.com/Comcast/talaria/releases/download/v0.0.1-65alpha/RPM-GPG-KEY-comcast-webpa
```

2. Install the rpm with yum (so it installs any/all dependencies for you)

```
yum install https://github.com/Comcast/talaria/releases/download/v0.0.1-65alpha/talaria-0.0.1-65.el6.x86_64.rpm
```
