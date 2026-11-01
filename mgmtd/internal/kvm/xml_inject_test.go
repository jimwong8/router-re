package kvm

import (
	"encoding/xml"
	"strings"
	"testing"
)

// Network 直接进 <source network='...'/>，以前完全没校验。
// 带引号就能闭合属性并在 <devices> 里插入任意 XML 元素
// ——<hostdev> 可以把宿主机 PCI 设备直通给 guest，越出虚拟化边界。
func TestValidateCreateRejectsNetworkInjection(t *testing.T) {
	payloads := []string{
		"default'/><console type='file' path='/etc/passwd",
		"default'/><hostdev mode='subsystem' type='pci' managed='yes'/>",
		"default\nPWNED",
		"default\r\nPWNED",
		"default;reboot",
		"../etc",
		"default network",
	}
	for _, p := range payloads {
		s := CreateSpec{Name: "vm1", VCPU: 2, MemoryMiB: 512, DiskPath: "/etc/hostname", Network: p}
		if err := ValidateCreate(s); err == nil {
			t.Errorf("Network=%q 未被拒绝", p)
		}
	}
}

// 即使调用方跳过 ValidateCreate，渲染层也必须让注入值只是一段字符串。
func TestRenderXMLNoInjection(t *testing.T) {
	payloads := []string{
		"default'/><console type='file' path='/etc/passwd",
		"default'/><hostdev mode='subsystem' type='pci' managed='yes'/>",
		"default\nPWNED",
	}
	for _, p := range payloads {
		s := CreateSpec{Name: "vm1", VCPU: 2, MemoryMiB: 512, DiskPath: "/tmp/a.qcow2", Network: p}
		out := RenderXML(s)
		if strings.Contains(out, "<hostdev") || strings.Contains(out, "<console type='file'") {
			t.Fatalf("Network=%q 注入了新元素:\n%s", p, out)
		}
		// 必须以合法 XML 解析成功，且 interface 下只有 source+model
		var dom struct {
			XMLName xml.Name
			Devices struct {
				Interfaces []struct {
					Source struct {
						Network string `xml:"network,attr"`
					} `xml:"source"`
				} `xml:"interface"`
			} `xml:"devices"`
		}
		if err := xml.Unmarshal([]byte(out), &dom); err != nil {
			t.Fatalf("Network=%q 时 XML 非法: %v\n%s", p, err, out)
		}
		if got := dom.Devices.Interfaces[0].Source.Network; got != "default" {
			t.Errorf("非法网络名应回落到 default，实际 %q", got)
		}
	}
}

func TestRenderXMLAcceptsLegit(t *testing.T) {
	s := CreateSpec{
		Name: "web-01", VCPU: 4, MemoryMiB: 2048,
		DiskPath: "/var/lib/libvirt/images/web.qcow2",
		ISO:      "/var/lib/libvirt/images/seed.iso",
		Network:  "lan-2",
	}
	out := RenderXML(s)
	for _, want := range []string{
		"<name>web-01</name>",
		"<source file='/var/lib/libvirt/images/web.qcow2'/>",
		"<source file='/var/lib/libvirt/images/seed.iso'/>",
		"<source network='lan-2'/>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
}

// 控制字符在 XML 1.0 里非法，原样写进去会让 libvirt 解析失败
// ——一次请求就能让这台虚机永远定义不出来。
func TestRenderXMLStripsControlChars(t *testing.T) {
	s := CreateSpec{Name: "vm1", VCPU: 2, MemoryMiB: 512,
		DiskPath: "/tmp/a\x00b\x0b.qcow2", Network: "default"}
	out := RenderXML(s)
	for _, bad := range []string{"\x00", "\x0b"} {
		if strings.Contains(out, bad) {
			t.Errorf("控制字符 %q 未剔除", bad)
		}
	}
}

// 名字里的特殊字符必须转义，不能破坏文档结构。
func TestRenderXMLEscapesName(t *testing.T) {
	s := CreateSpec{Name: "vm<&>1", VCPU: 2, MemoryMiB: 512, DiskPath: "/tmp/a.qcow2"}
	out := RenderXML(s)
	if strings.Contains(out, "<name>vm<&>1</name>") {
		t.Fatalf("名字未转义:\n%s", out)
	}
	if !strings.Contains(out, "&lt;") {
		t.Errorf("期望出现转义实体:\n%s", out)
	}
}
