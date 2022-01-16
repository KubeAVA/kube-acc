# kube-gpu

## Authors

## Running

Switch to the project root directory.

### Install GPU CRD
```
kubectl apply -f ./deploy/doslab.io_gpus.yaml
```

### Make kube-gpu local binary
```
make
```

### Make kube-gpu docker image
```
docker build -t registry.cn-beijing.aliyuncs.com/doslab/kube-gpu:v0.2.0-amd64 .
```

### Run kube-gpu DaemonSet
```
kubectl apply -f ./deploy/kube-gpu.yaml
```

### Run demo
```
kubectl apply -f ./example/po-1.yaml
```
