# kube-gpu

## Authors

- luorongzhou20@otcaix.iscas.ac.cn
- wuheng@otcaix.iscas.ac.cn

## Running

Switch to the project root directory and modify **masterUrl** and **token** in `./deploy/kube-gpu.yaml`, more details in https://github.com/kubesys/client-go

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
