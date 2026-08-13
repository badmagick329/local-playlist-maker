PROJECT_PATH := ./src/PlaylistMaker.Console/PlaylistMaker.csproj
TUI_PROJECT_PATH := ./src/PlaylistMaker.Tui/PlaylistMaker.Tui.csproj
PROJECT_NAME := PlaylistMaker
CONFIGURATION := Release
OUTPUT_DIR := ./publish

all: win osx linux

win:
	@echo "Building for Windows (win-x64)..."
	dotnet publish $(PROJECT_PATH) -c $(CONFIGURATION) -r win-x64 --output $(OUTPUT_DIR)/windows --self-contained -p:PublishSingleFile=true

osx:
	@echo "Building for macOS (osx-x64)..."
	dotnet publish $(PROJECT_PATH) -c $(CONFIGURATION) -r osx-x64 --output $(OUTPUT_DIR)/osx --self-contained -p:PublishSingleFile=true

linux:
	@echo "Building for Linux (linux-x64)..."
	dotnet publish $(PROJECT_PATH) -c $(CONFIGURATION) -r linux-x64 --output $(OUTPUT_DIR)/linux --self-contained -p:PublishSingleFile=true

clean:
	@echo "Cleaning up..."
	rm -rf $(OUTPUT_DIR)

run:
	@echo "Running with arguments: $(ARGS)"
	dotnet run --project $(PROJECT_PATH) -- $(ARGS)

run-legacy:
	dotnet run --project $(PROJECT_PATH) -- $(ARGS)

run-tui:
	dotnet run --project $(TUI_PROJECT_PATH) -- $(ARGS)

publish-tui-win:
	dotnet publish $(TUI_PROJECT_PATH) -c $(CONFIGURATION) -r win-x64 --output $(OUTPUT_DIR)/tui-windows --self-contained -p:PublishSingleFile=true

.PHONY: all win osx linux clean run run-legacy run-tui publish-tui-win
