APP := agentteam

.PHONY: fmt test run build

fmt:
	gofmt -w ./cmd ./pkg

test:
	go test ./...

build:
	go build -o bin/$(APP) ./cmd/agentteam

run:
	go run ./cmd/agentteam run --team ./examples/software-team/team.yaml --task "Launch the MVP with confidence"
