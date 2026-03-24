# vmdk-utils

WSL2/Linux 下的 VMDK 查看、挂载、修改与重打包工具。

当前版本是 V1 CLI 骨架，主后端为 `qemu-nbd`。

现在支持：
- 自动分区探测
- LVM PV -> VG/LV 激活与挂载

## 当前已实现

- `vmdkctl inspect <image>`
- `vmdkctl attach <image>`
- `vmdkctl mount <image> <mountpoint>`
- `vmdkctl umount <mountpoint>`
- `vmdkctl pull <image> <guest-path> <local-path>`
- `vmdkctl push <image> <local-path> <guest-path>`
- `vmdkctl repack <src-image> <dst.vmdk>`
- `vmdkctl cleanup`
- `vmdkctl detach <image|device>`
- `vmdkctl status`

## 计划中的命令


## 运行前提

- Linux / WSL2
- 可用的 `qemu-img`
- 可用的 `qemu-nbd`
- 已存在 `/dev/nbdX`

项目会优先使用 `runtime/bin` 下自带的二进制；若不存在，则回退到系统 `PATH`。

## 打包 runtime

可用脚本把当前系统中的依赖复制到项目内：

```bash
./scripts/package-runtime.sh
```

默认会打包：
- `qemu-img`
- `qemu-nbd`
- `partprobe`
- `lsblk`
- `blkid`
- `mount`
- `umount`
- `pvs`
- `lvs`
- `vgchange`

## 示例

```bash
go run ./cmd/vmdkctl --help
go run ./cmd/vmdkctl inspect disk.vmdk
go run ./cmd/vmdkctl attach --device /dev/nbd0 disk.vmdk
go run ./cmd/vmdkctl mount disk.vmdk /mnt/vmdk
go run ./cmd/vmdkctl mount --partition 1 disk.vmdk /mnt/vmdk
go run ./cmd/vmdkctl pull disk.vmdk /etc/fstab ./fstab
go run ./cmd/vmdkctl push disk.vmdk ./hosts /etc/hosts
go run ./cmd/vmdkctl repack --profile workstation disk.qcow2 disk.vmdk
go run ./cmd/vmdkctl status
go run ./cmd/vmdkctl cleanup
go run ./cmd/vmdkctl umount /mnt/vmdk
go run ./cmd/vmdkctl detach /dev/nbd0
```

## 状态文件

会在以下位置记录已追踪会话：

```text
~/.local/state/vmdkctl/sessions.json
```

## 下一步

1. 丰富 `inspect` 结构化输出
2. 增加集成测试样本
3. 增强 runtime 打包覆盖面
