shotgun-go
==========

A port of the rack reloading tool shotgun to go.

Using
------
Example Usage:
`shotgun -u http://localhost:8008 -p 8010 -checkCmd='exit `+"`find -name *.go -newer ./bin/myapp | wc -l`"+`' -buildCmd="go build -o ./bin/myapp myapp" -runCmd="./bin/myapp"`

Or simply use a yml config file named `.shotgun-go` and run shotgun
```yml
env:
  - FOO: "bar"
port: 8010
url: http://localhost:8008
checkcmd: "exit `+"`find -name *.go -newer ./bin/myapp | wc -l`"+`"
buildcmd: "go build -o ./bin/myapp myapp"
runcmd: "./bin/myapp"
```
