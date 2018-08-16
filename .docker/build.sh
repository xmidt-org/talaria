GOOS=linux GOARCH=amd64 go build ../src/talaria/

docker build -t talaria:local .
