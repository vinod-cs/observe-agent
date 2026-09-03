//go:build linux

// AGENTV1 FILE START: explicit Linux-only metric reads; no cpu package init-time sampling.
package platform

import (
	"context"
	"errors"
	"github.com/agent-i/agent/internal/config"
	"github.com/shirou/gopsutil/v4/common"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type linuxMetrics struct {
	mu       sync.Mutex
	limits   config.Collection
	previous []float64
	last     time.Time
}

func NewHostMetrics(limits config.Collection) (HostMetrics, error) {
	return &linuxMetrics{limits: limits}, nil
}

// CounterRatio omits first sample, reset, non-progress and gaps; idle includes I/O wait.
func CounterRatio(previous, current []float64, gap time.Duration) (float64, bool) {
	if len(previous) != 8 || len(current) != 8 || gap <= 0 || gap > 120*time.Second {
		return 0, false
	}
	total, idle := 0.0, 0.0
	for i := range current {
		d := current[i] - previous[i]
		if d < 0 {
			return 0, false
		}
		total += d
		if i == 3 || i == 4 {
			idle += d
		}
	}
	if total <= 0 {
		return 0, false
	}
	return 100 * (total - idle) / total, true
}
func procCPU() ([]float64, time.Time, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 2<<20+1))
	if err != nil || len(b) > 2<<20 {
		return nil, time.Time{}, errors.New("CPU counters unavailable")
	}
	var values []float64
	var boot time.Time
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 9 && fields[0] == "cpu" {
			values = make([]float64, 8)
			for i := range values {
				n, e := strconv.ParseUint(fields[i+1], 10, 64)
				if e != nil {
					return nil, time.Time{}, e
				}
				values[i] = float64(n) / 100
			}
		}
		if len(fields) == 2 && fields[0] == "btime" {
			n, e := strconv.ParseInt(fields[1], 10, 64)
			if e == nil && n > 0 {
				boot = time.Unix(n, 0)
			}
		}
	}
	if len(values) != 8 {
		return nil, time.Time{}, errors.New("CPU counters absent")
	}
	return values, boot, nil
}
func (m *linuxMetrics) Sample(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	// Pin host roots; inherited HOST_* variables cannot redirect disabled-log-safe reads.
	ctx = context.WithValue(ctx, common.EnvKey, common.EnvMap{common.HostProcEnvKey: "/proc", common.HostSysEnvKey: "/sys", common.HostEtcEnvKey: "/etc", common.HostRunEnvKey: "/run", common.HostDevEnvKey: "/dev", common.HostVarEnvKey: "/var", common.HostRootEnvKey: "/", common.HostProcMountinfo: "/proc/self/mountinfo"})
	s := Snapshot{ObservedAt: time.Now().UTC()}
	issue := func(cap string) { s.Issues = append(s.Issues, Issue{cap, "unavailable_or_denied"}) }
	add := func(name, unit, kind string, value float64, attrs map[string]string) {
		if len(s.Values) >= m.limits.MaxPoints {
			return
		}
		if kind == "sum" && s.StartTime.IsZero() {
			return
		}
		s.Values = append(s.Values, Measurement{name, unit, kind, value, attrs})
	}
	cpu, boot, err := procCPU()
	if err != nil {
		issue("cpu")
	} else {
		s.StartTime = boot
		if boot.IsZero() {
			issue("counter_origin")
		}
		for i, state := range []string{"user", "nice", "system", "idle", "wait", "interrupt", "softirq", "steal"} {
			add("system.cpu.time", "s", "sum", cpu[i], map[string]string{"state": state})
		}
		if value, ok := CounterRatio(m.previous, cpu, s.ObservedAt.Sub(m.last)); ok {
			add("host.cpu.used_pct", "%", "gauge", value, nil)
			add("system.cpu.utilization", "1", "gauge", value/100, map[string]string{"state": "used"})
		}
		m.previous = cpu
		m.last = s.ObservedAt
	}
	if v, e := mem.VirtualMemoryWithContext(ctx); e != nil || v.Total == 0 || v.Available > v.Total {
		issue("memory")
	} else {
		used := v.Total - v.Available
		add("host.memory.used_pct", "%", "gauge", float64(used)/float64(v.Total)*100, nil)
		add("system.memory.usage", "By", "gauge", float64(used), map[string]string{"state": "used"})
		add("system.memory.usage", "By", "gauge", float64(v.Available), map[string]string{"state": "available"})
		add("system.memory.utilization", "1", "gauge", float64(used)/float64(v.Total), map[string]string{"state": "used"})
	}
	if v, e := load.AvgWithContext(ctx); e != nil {
		issue("load")
	} else {
		add("system.cpu.load_average.1m", "1", "gauge", v.Load1, nil)
		add("system.cpu.load_average.5m", "1", "gauge", v.Load5, nil)
		add("system.cpu.load_average.15m", "1", "gauge", v.Load15, nil)
	}
	if devices, e := disk.IOCountersWithContext(ctx); e != nil {
		issue("disk")
	} else {
		names := make([]string, 0, len(devices))
		for n := range devices {
			names = append(names, n)
		}
		sort.Strings(names)
		for i, n := range names {
			if i >= m.limits.MaxDevices {
				issue("disk_cardinality")
				break
			}
			v := devices[n]
			for _, d := range []struct {
				direction                string
				bytes, ops, milliseconds uint64
			}{{"read", v.ReadBytes, v.ReadCount, v.ReadTime}, {"write", v.WriteBytes, v.WriteCount, v.WriteTime}} {
				a := map[string]string{"device": n, "direction": d.direction}
				add("system.disk.io", "By", "sum", float64(d.bytes), a)
				add("system.disk.operations", "{operations}", "sum", float64(d.ops), a)
				add("system.disk.operation_time", "s", "sum", float64(d.milliseconds)/1000, a)
			}
			add("system.disk.pending_operations", "{operations}", "gauge", float64(v.IopsInProgress), map[string]string{"device": n})
		}
	}
	if interfaces, e := gnet.IOCountersWithContext(ctx, true); e != nil {
		issue("network")
	} else {
		sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].Name < interfaces[j].Name })
		for i, v := range interfaces {
			if i >= m.limits.MaxInterfaces {
				issue("network_cardinality")
				break
			}
			for _, d := range []struct {
				direction                   string
				bytes, packets, errs, drops uint64
			}{{"receive", v.BytesRecv, v.PacketsRecv, v.Errin, v.Dropin}, {"transmit", v.BytesSent, v.PacketsSent, v.Errout, v.Dropout}} {
				a := map[string]string{"device": v.Name, "direction": d.direction}
				add("system.network.io", "By", "sum", float64(d.bytes), a)
				add("system.network.packets", "{packets}", "sum", float64(d.packets), a)
				add("system.network.errors", "{errors}", "sum", float64(d.errs), a)
				add("system.network.dropped", "{packets}", "sum", float64(d.drops), a)
			}
		}
	}
	if mounts, e := disk.PartitionsWithContext(ctx, true); e != nil {
		issue("filesystem")
	} else {
		sort.Slice(mounts, func(i, j int) bool { return mounts[i].Mountpoint < mounts[j].Mountpoint })
		seen := map[string]bool{}
		count := 0
		for _, mount := range mounts {
			if !localFilesystem(mount.Fstype) || seen[mount.Mountpoint] {
				continue
			}
			seen[mount.Mountpoint] = true
			if count >= m.limits.MaxMounts {
				issue("filesystem_cardinality")
				break
			}
			count++
			if err := ctx.Err(); err != nil {
				return s, err
			}
			v, e := disk.UsageWithContext(ctx, mount.Mountpoint)
			if e != nil {
				issue("filesystem")
				continue
			}
			for _, state := range []struct {
				name  string
				value uint64
			}{{"used", v.Used}, {"available", v.Free}} {
				add("system.filesystem.usage", "By", "gauge", float64(state.value), map[string]string{"device": mount.Device, "mountpoint": mount.Mountpoint, "type": mount.Fstype, "state": state.name})
			}
		}
	}
	if len(s.Values) >= m.limits.MaxPoints {
		issue("sample_cardinality")
	}
	return s, nil
}
func localFilesystem(kind string) bool {
	switch kind {
	case "ext2", "ext3", "ext4", "xfs", "btrfs", "zfs", "tmpfs", "overlay":
		return true
	}
	return false
}

// AGENTV1 FILE END
