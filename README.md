# kube-gpu

## Running

Switch to the project root directory.

### Install GPU CRD
```
kubectl apply -f ./doslab.io_gpus.yaml
```

### Make kube-gpu local binary
```
make
```

### Make kube-gpu docker image
```
docker build -t doslab/kube-sched:v0.1-amd64 .
```

### Run kube-gpu DaemonSet
```
kubectl apply -f ./deploy/kube-gpu.yaml
```

### Run demo
```
kubectl apply -f ./example/po-1.yaml
```