TARGET=kube-gpu
GO=go
BIN_DIR=bin/
GO_MODULE=GO111MODULE=on
FLAGS=CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files' GOOS=linux GOARCH=amd64

.PHONY: all clean $(TARGET)

all: $(TARGET)

kube-gpu:
	$(GO_MODULE) $(FLAGS) $(GO) build -o $(BIN_DIR)$@

clean:
	rm $(BIN_DIR)* 2>/dev/null; exit 0