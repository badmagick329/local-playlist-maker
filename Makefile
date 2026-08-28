CHARM_PROJECT_PATH := ./src/PlaylistMaker.Charm

build:
	cd $(CHARM_PROJECT_PATH) && go build ./cmd/playlistmaker-charm

clean:
	cd $(CHARM_PROJECT_PATH) && go clean

run:
	cd $(CHARM_PROJECT_PATH) && go run ./cmd/playlistmaker-charm --config ../../config.yaml $(ARGS)

run-charm:
	powershell -NoProfile -File ./scripts/run-charm.ps1

test:
	cd $(CHARM_PROJECT_PATH) && go test ./...

vet:
	cd $(CHARM_PROJECT_PATH) && go vet ./...

.PHONY: build clean run run-charm test vet
