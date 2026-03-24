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
- `vmdkctl mount-all <image> <mount-root>`
- `vmdkctl umount <mountpoint>`
- `vmdkctl pull <image> <guest-path> <local-path>`
- `vmdkctl push <image> <local-path> <guest-path>`
- `vmdkctl repack <src-image> <dst.vmdk>`
- `vmdkctl cleanup`
- `vmdkctl detach <image|device>`
- `vmdkctl status`
- `vmdkctl detect-deps`

## 计划中的命令


## 运行前提

- Linux / WSL2
- 可用的 `qemu-img`
- 可用的 `qemu-nbd`
- 可用的 `partprobe`
- 可用的 `lsblk`
- 可用的 `mount` / `umount`
- 可用的 `modprobe`（工具会尝试自动加载 `nbd max_part=16`；失败时再手动执行）
- 如需 LVM 支持：可用的 `blkid`、`pvs`、`lvs`、`vgchange`
- 宿主机内核支持 `nbd`

项目直接使用系统 `PATH` 中的工具。

## 依赖检测

可用命令检查依赖是否齐全：

```bash
go run ./cmd/vmdkctl detect-deps
```

## 依赖安装

推荐直接安装系统包，由包管理器自动带上所需共享库依赖。

### Ubuntu / Debian

```bash
sudo apt update
sudo apt install -y qemu-utils parted util-linux lvm2 kmod
```

对应关系：
- `qemu-img` / `qemu-nbd` -> `qemu-utils`
- `partprobe` -> `parted`
- `lsblk` / `mount` / `umount` / `blkid` -> `util-linux`
- `pvs` / `lvs` / `vgchange` -> `lvm2`
- `modprobe` -> `kmod`

### Fedora / RHEL / Rocky / AlmaLinux

```bash
sudo dnf install -y qemu-img qemu-nbd parted util-linux lvm2 kmod
```

### Arch Linux

```bash
sudo pacman -S --needed qemu parted util-linux lvm2 kmod
```

### 内核能力

- 还需要宿主机内核支持 `nbd`
- 加载方式：`sudo modprobe nbd max_part=16`

### 关于系统库

- 当前项目不再单独打包 `.so` 共享库，而是直接使用系统安装的工具
- 这些工具依赖的系统库通常会被 `apt` / `dnf` / `pacman` 自动作为传递依赖安装
- 因为不同发行版的库名和版本差异很大，README 不单独硬编码列出每个 `.so` 名称
- 实际上需要关心的是系统包是否装齐；装齐后共享库一般也会一起就绪

## 示例

```bash
go run ./cmd/vmdkctl --help
go run ./cmd/vmdkctl inspect disk.vmdk
go run ./cmd/vmdkctl attach --device /dev/nbd0 disk.vmdk
go run ./cmd/vmdkctl mount disk.vmdk /mnt/vmdk
go run ./cmd/vmdkctl mount-all disk.vmdk /mnt/vmdk-all
go run ./cmd/vmdkctl mount --partition 1 disk.vmdk /mnt/vmdk
go run ./cmd/vmdkctl pull disk.vmdk /etc/fstab ./fstab
go run ./cmd/vmdkctl push disk.vmdk ./hosts /etc/hosts
go run ./cmd/vmdkctl repack --profile workstation disk.qcow2 disk.vmdk
go run ./cmd/vmdkctl status
go run ./cmd/vmdkctl detect-deps
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
3. 增加更多自动化测试
