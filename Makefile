TARGET=kube-gpu
GO=go
BIN_DIR=bin/

.PHONY: all clean $(TARGET)

all: $(TARGET)

kube-gpu:
	$(GO) build -o $(BIN_DIR)$@

clean:
	rm $(BIN_DIR)* 2>/dev/null; exit 0