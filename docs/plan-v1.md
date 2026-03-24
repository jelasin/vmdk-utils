# V1 CLI 计划

## 目标

第一版提供一个可运行的 CLI 骨架，围绕 `qemu-nbd` 建立基础能力。

## 已落地范围

- 命令分发框架
- `inspect`
- `attach`
- `detach`
- `status`
- 本地状态存储
- 系统 `PATH` 依赖解析

## 下一阶段

### M2
- `mount`
- `umount`
- 分区等待逻辑
- 挂载点状态记录

### M3
- `pull`
- `push`

### 已完成
- `inspect` 增强为附带块设备信息
- `pull`
- `push`
- 临时挂载工作流

### M4
- `repack`
- 导出 profile

### M5
- 错误恢复
- stale session 清理
- 基础集成测试

### 已完成
- `repack`
- VMDK 导出 profile: workstation / esxi / stream-optimized
- 自动分区探测
- LVM 激活/回收
- stale session cleanup
- `detect-deps` 依赖检测
