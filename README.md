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
- `vmdkctl convert <src-image> <dst.vmdk>`
- `vmdkctl repack <src-image> <dst.vmdk>`
- `vmdkctl cleanup`
- `vmdkctl detach <image|device>`
- `vmdkctl status`
- `vmdkctl detect-deps`

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

## 构建

```bash
go build ./cmd/vmdkctl
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

## 常见挂载修改流程

强烈建议先复制一份工作镜像，再对副本做读写挂载。`--rw` 不会自动复制 VMDK。

```bash
cp PA-VM-ESX-11.1.4-disk1.vmdk work.vmdk
sudo ./vmdkctl inspect work.vmdk
sudo ./vmdkctl mount-all --rw work.vmdk ./mnt-work

# 修改 ./mnt-work/p1、p2、p3、p5、p6、p8 中的文件
# p4 是扩展分区容器，p7 是 swap，通常不会被挂载

sudo ./vmdkctl umount ./mnt-work
sudo ./vmdkctl repack --profile workstation work.vmdk final.vmdk
sudo ./vmdkctl inspect final.vmdk
```

如果只需要写入一个文件，可以用 `push`，它会临时读写挂载目标分区，复制完成后自动卸载：

```bash
cp disk.vmdk work.vmdk
sudo ./vmdkctl push --partition 5 work.vmdk ./local-config.xml /opt/pancfg/mgmt/saved-configs/config.xml
sudo ./vmdkctl repack --profile workstation work.vmdk final.vmdk
```

如果不指定 `--partition`，`push` 会使用自动探测出的根分区。多系统分区镜像中，建议显式指定目标分区。

## 修改与重打包原理

本工具不是把 VMDK 解包成目录、修改目录后再重新造盘。实际路径是：

```text
VMDK 文件
-> qemu-nbd 映射为 /dev/nbdX
-> 内核识别 /dev/nbdXpN 分区
-> mount 分区文件系统
-> 文件修改通过块设备写回 VMDK
```

因此：

- 默认 `mount` / `mount-all` 是只读，不会修改源镜像
- 加 `--rw` 后会直接修改传入的那个 VMDK 文件
- `umount` 后，文件系统缓存会刷回，VMDK 已经包含修改
- `repack` 不从挂载目录重建磁盘，而是用 `qemu-img convert` 基于已修改的整盘镜像生成新 VMDK
- `repack` 会先写临时 VMDK，再只读 attach 并验证可挂载文件系统，成功后才安装为目标文件

`repack` 的目标是保留原镜像的块级磁盘布局，包括 MBR、分区表、文件系统 UUID、label、boot sector 和各分区内容。这样比从目录重新创建分区和文件系统更可靠。

注意事项：

- 对重要镜像修改前先复制，避免 `--rw` 直接改坏原文件
- 修改期间不要对同一个镜像同时运行 `convert` / `repack`
- 修改完成必须执行 `umount`，不要直接删除挂载目录或断开 NBD
- PAN-OS 等镜像可能有双系统分区，例如 `sysroot0` / `sysroot1`，只改一个分区不一定对当前启动系统生效
- `mount-all` 只挂载可挂载文件系统分区，扩展分区和 swap 会被跳过

## 命令参考

### `inspect`

查看 VMDK 元数据、临时 attach 镜像、列出块设备和分区信息，并给出自动识别的根分区候选。

```bash
sudo ./vmdkctl inspect disk.vmdk
sudo ./vmdkctl inspect --json disk.vmdk
```

选项：

- `--json`：只输出 `qemu-img info --output=json` 的结果

说明：

- 默认不会修改源镜像
- 非 `--json` 模式会临时只读 attach 镜像，结束后自动 detach
- 输出中的 `Suggested root target` 是 `mount` / `push` 未指定 `--partition` 时的自动选择参考

### `attach`

把 VMDK 映射到 `/dev/nbdX`，但不自动挂载文件系统。

```bash
sudo ./vmdkctl attach disk.vmdk
sudo ./vmdkctl attach --device /dev/nbd0 disk.vmdk
sudo ./vmdkctl attach --rw disk.vmdk
```

选项：

- `--device /dev/nbdX`：指定 NBD 设备；不指定时自动选择空闲设备
- `--read-only`：只读 attach，默认值
- `--rw`：读写 attach，会允许后续通过块设备修改源 VMDK

说明：

- `attach` 会记录会话状态，后续可用 `status` 查看
- 使用完需要执行 `detach <image|device>`，或者通过对应挂载命令的 `umount` 清理

### `mount`

挂载一个文件系统分区到指定目录。

```bash
sudo ./vmdkctl mount disk.vmdk /mnt/vmdk
sudo ./vmdkctl mount --partition 5 disk.vmdk /mnt/pancfg
sudo ./vmdkctl mount --rw --partition 2 work.vmdk /mnt/sysroot0
```

选项：

- `--device /dev/nbdX`：复用或指定 NBD 设备
- `--partition N`：挂载指定分区；不指定时自动探测根分区
- `--read-only`：只读挂载，默认值
- `--rw`：读写挂载，会直接修改传入的 VMDK

说明：

- 只挂载一个目标分区
- 对 ext3/ext4 只读挂载失败时，会尝试使用 `ro,noload` 方式恢复挂载
- 修改完成后必须执行 `sudo ./vmdkctl umount <mountpoint>`

### `mount-all`

挂载镜像中所有可挂载的文件系统分区到一个目录下的子目录。

```bash
sudo ./vmdkctl mount-all disk.vmdk ./mnt-all
sudo ./vmdkctl mount-all --rw work.vmdk ./mnt-work
```

选项：

- `--device /dev/nbdX`：复用或指定 NBD 设备
- `--read-only`：只读挂载，默认值
- `--rw`：读写挂载，会直接修改传入的 VMDK

说明：

- 子目录名通常是 `p1`、`p2`、`p5` 等
- 只挂载有文件系统的分区；扩展分区和 swap 会跳过
- 会写入 `.vmdkctl-mount-all.json` manifest，用于后续 `umount`
- 修改完成后执行 `sudo ./vmdkctl umount <mount-root>`

### `umount`

卸载由 `mount` 或 `mount-all` 建立的挂载，并 detach 对应 NBD 设备。

```bash
sudo ./vmdkctl umount /mnt/vmdk
sudo ./vmdkctl umount ./mnt-work
```

说明：

- 对单分区挂载，会卸载该 mountpoint 并 detach NBD
- 对 `mount-all`，会按 manifest 卸载所有子目录、清理子目录、detach NBD
- 对读写挂载，`umount` 完成后修改已经刷回 VMDK

### `pull`

从镜像里的文件系统复制文件或目录到本机。

```bash
sudo ./vmdkctl pull disk.vmdk /etc/fstab ./fstab
sudo ./vmdkctl pull --partition 5 disk.vmdk /opt/pancfg ./pancfg
```

选项：

- `--device /dev/nbdX`：复用或指定 NBD 设备
- `--partition N`：从指定分区读取；不指定时自动探测根分区

说明：

- 临时只读挂载，复制完成后自动卸载和 detach
- 不会修改源 VMDK

### `push`

把本机文件或目录复制进镜像里的文件系统。

```bash
sudo ./vmdkctl push work.vmdk ./hosts /etc/hosts
sudo ./vmdkctl push --partition 5 work.vmdk ./config.xml /opt/pancfg/mgmt/saved-configs/config.xml
```

选项：

- `--device /dev/nbdX`：复用或指定 NBD 设备
- `--partition N`：写入指定分区；不指定时自动探测根分区

说明：

- 会临时读写挂载目标分区
- 会直接修改传入的 VMDK，所以建议对副本执行
- 复制完成后自动卸载和 detach

### `convert`

使用 `qemu-img convert` 做通用镜像格式转换。

```bash
./vmdkctl convert --to qcow2 disk.vmdk disk.qcow2
./vmdkctl convert --from qcow2 --to vmdk --profile workstation disk.qcow2 disk.vmdk
```

选项：

- `--to <format>`：目标格式，必填，例如 `vmdk`、`qcow2`、`raw`
- `--from <format>`：源格式，可选
- `--profile workstation|esxi|stream-optimized`：VMDK 输出 profile，仅 `--to vmdk` 时可用

说明：

- 不修改源镜像
- 不会主动 attach 校验目标镜像
- 如果源镜像正在读写挂载，不应同时执行 `convert`

### `repack`

把已经修改完成并卸载的镜像重新输出为一个新的、经过只读挂载校验的 VMDK。

```bash
sudo ./vmdkctl repack --profile workstation work.vmdk final.vmdk
sudo ./vmdkctl repack --force --profile workstation work.vmdk final.vmdk
```

选项：

- `--from <format>`：源格式，可选
- `--profile workstation|esxi|stream-optimized`：输出 VMDK profile，默认 `workstation`
- `--force`：目标文件已存在时覆盖

说明：

- 不修改源镜像
- 会先写临时 VMDK，校验通过后才替换为目标文件
- 会只读 attach 临时 VMDK，并验证可挂载文件系统
- 适合在 `--rw` 修改副本后，生成交付、归档或导入用的最终 VMDK
- 如果只是在本机继续使用修改后的 `work.vmdk`，可以不执行 `repack`

### `detach`

断开一个已 attach 的 NBD 设备，并从状态文件删除对应会话。

```bash
sudo ./vmdkctl detach /dev/nbd0
sudo ./vmdkctl detach disk.vmdk
```

说明：

- 可传 NBD 设备路径，也可传镜像路径
- 对已经 mount 的文件系统，应先用 `umount`，不要直接 `detach`

### `status`

查看当前记录的镜像、NBD 设备、挂载点和健康状态。

```bash
./vmdkctl status
```

说明：

- 可用于确认是否还有未卸载的会话
- `stale(...)` 表示状态文件里有过期记录，可用 `cleanup` 清理

### `cleanup`

清理状态文件里的过期会话记录。

```bash
./vmdkctl cleanup
./vmdkctl cleanup --force
```

选项：

- `--force`：不做健康检查，直接删除所有记录

说明：

- 默认只删除 stale 记录
- 不负责强制卸载真实挂载点；真实挂载仍应使用 `umount`

### `detect-deps`

检查运行所需的系统命令和内核 NBD 支持。

```bash
./vmdkctl detect-deps
```

说明：

- 会检查 `qemu-img`、`qemu-nbd`、`partprobe`、`lsblk`、`mount`、`umount`、`modprobe` 等
- 会提示 Ubuntu/Debian、Fedora/RHEL、Arch 对应安装包

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
go run ./cmd/vmdkctl convert --to vmdk --profile workstation disk.qcow2 disk.vmdk
go run ./cmd/vmdkctl convert --from vmdk --to qcow2 disk.vmdk disk.qcow2
go run ./cmd/vmdkctl repack --profile workstation modified.vmdk final.vmdk
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
