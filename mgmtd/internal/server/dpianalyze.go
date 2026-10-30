package server

import (
	"bytes"
	"io"
	"net/http"
	"sort"

	"github.com/jimwong/router-re/mgmtd/internal/appid"
	"github.com/jimwong/router-re/mgmtd/internal/dpi"
	"github.com/jimwong/router-re/mgmtd/internal/dpiflow"
)

// 抓包回放的流分析。
//
// 为什么要这个接口：nDPI 的价值全在"跨包累积"上，而这件事很难靠一句话
// 让人相信。上传一段真实抓包、当场看到每条流的协议与命中包数，是最直接
// 的证据；也方便在现场抓包后回来复盘"到底识别成什么"。

// maxPcapUpload 限制上传体积。
const maxPcapUpload = 32 << 20

// handleAnalyzePcap 接收 pcap，回放并返回按流聚合的结果。
func (s *Server) handleAnalyzePcap(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPcapUpload))
	if err != nil {
		writeJSON(w, 400, errBody(12000, "read body failed", err.Error()))
		return
	}
	if len(body) == 0 {
		writeJSON(w, 422, errBody(12001, "empty body", "没有收到数据，请上传 .pcap 文件"))
		return
	}

	pkts, err := dpi.ReadPcap(bytes.NewReader(body))
	if err != nil {
		writeJSON(w, 422, errBody(12002, "invalid pcap", err.Error()))
		return
	}
	if len(pkts) == 0 {
		writeJSON(w, 422, errBody(12003, "no packets", "文件里没有可分析的 IP 包（支持 pcap / pcapng）"))
		return
	}

	dpi.ResetFlows()
	dpi.ResetQUICStreams()
	defer dpi.ResetFlows()
	defer dpi.ResetQUICStreams()

	flows := dpi.AnalyzePcap(pkts, dpi.ClassifyFlow)

	// 再补一层"这是哪个应用"：从流首包载荷里取 SNI / Host，查特征库。
	type enriched struct {
		dpi.FlowResult
		App      string `json:"app,omitempty"`
		AppCat   string `json:"app_category,omitempty"`
		Hostname string `json:"host,omitempty"`
	}
	out := make([]enriched, 0, len(flows))
	identified, appMatched := 0, 0
	// 按指纹归并：同一客户端连不同站点，SNI 不同但 JA4 相同。
	// 这是「这台设备在用什么软件」这件事的唯一线索——尤其在没有 SNI 的时候。
	fpIndex := map[string]*fpSummary{}
	var fpOrder []string
	for _, f := range flows {
		if f.Protocol != "Unknown" {
			identified++
		}
		e := enriched{FlowResult: f}
		if h := sniffHostForFlow(pkts, f); h != "" {
			e.Hostname = h
			if a, ok := appid.Identify(h); ok {
				e.App = a.Name
				e.AppCat = string(a.Category)
				appMatched++
			}
		}
		if f.JA4 != "" {
			fp, ok := fpIndex[f.JA4]
			if !ok {
				fp = &fpSummary{
					JA4:    f.JA4,
					JA3:    f.JA3,
					Client: f.Client,
					ALPN:   f.ALPN,
				}
				fpIndex[f.JA4] = fp
				fpOrder = append(fpOrder, f.JA4)
			}
			fp.Flows++
			fp.Packets += f.Packets
			if e.Hostname != "" && len(fp.Hosts) < 3 && !containsStr(fp.Hosts, e.Hostname) {
				fp.Hosts = append(fp.Hosts, e.Hostname)
			}
		}
		out = append(out, e)
	}

	fps := make([]*fpSummary, 0, len(fpOrder))
	for _, k := range fpOrder {
		fps = append(fps, fpIndex[k])
	}
	sort.SliceStable(fps, func(i, j int) bool {
		if fps[i].Flows != fps[j].Flows {
			return fps[i].Flows > fps[j].Flows
		}
		return fps[i].JA4 < fps[j].JA4
	})

	writeJSON(w, 200, map[string]interface{}{
		"packets":      len(pkts),
		"flows":        out,
		"flow_count":   len(flows),
		"identified":   identified,
		"app_matched":  appMatched,
		"fingerprints": fps,
		"fp_known":     dpi.KnownClientCount(),
		"flow_table":   dpi.Stats(),
		"quic_streams": dpi.QUICStreamStats(),
		"note": "识别来自 nDPI 的跨包累积：单包往往只能得到 Unknown，同一连接的后续包才会定性。" +
			"应用名来自特征库（域名/SNI -> 应用），库为空时只显示协议不显示应用。",
	})
}

// fpSummary 是按 JA4 归并后的指纹视图。
type fpSummary struct {
	JA4     string   `json:"ja4"`
	JA3     string   `json:"ja3,omitempty"`
	Client  string   `json:"client,omitempty"`
	ALPN    string   `json:"alpn,omitempty"`
	Flows   int      `json:"flows"`
	Packets int      `json:"packets"`
	Hosts   []string `json:"hosts,omitempty"`
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// handleDPIStats 返回 nDPI 流表状态。
func (s *Server) handleDPIStats(w http.ResponseWriter, r *http.Request) {
	l := appid.Get()
	apps := 0
	if l != nil {
		apps = l.Stats().Apps
	}
	writeJSON(w, 200, map[string]interface{}{
		"flow_table":   dpi.Stats(),
		"quic_streams": dpi.QUICStreamStats(),
		"appid_loaded": apps > 0,
		"appid_apps":   apps,
	})
}

// sniffHostForFlow 在这条流的包里找 SNI / HTTP Host / DNS 名。
func sniffHostForFlow(pkts []dpi.PCAPPacket, f dpi.FlowResult) string {
	for _, p := range pkts {
		payload := l4Payload(p.IP)
		if payload == nil {
			continue
		}
		// 只关心这个五元组
		sp, dp := flowPorts(p.IP)
		if !sameTuple(sp, dp, f.SrcPort, f.DstPort) {
			continue
		}
		// QUIC（HTTP/3）的 SNI 在加密的 Initial 包里，dpiflow 的明文嗅探看不到，
		// 必须走 Initial 解密 + CRYPTO 重组这条路径。
		if dp == 443 || sp == 443 {
			if sni := dpi.QUICSNI(payload); sni != "" {
				return sni
			}
		}
		app, _ := dpiflow.DetectApp(payload, int(dp))
		for _, prefix := range []string{"tls-sni:", "http:", "dns:"} {
			if len(app) > len(prefix) && app[:len(prefix)] == prefix {
				h := app[len(prefix):]
				// 载荷嗅探对二进制流会误判：把一段随机字节当作 DNS 名返回。
				// 域名必须过基本校验，否则界面上的"应用"会变成一串乱码。
				if validHostname(h) {
					return h
				}
			}
		}
	}
	return ""
}

// validHostname 校验一个看起来像域名的字符串：只允许字母数字点短横线，
// 必须含点，且至少有一个非数字的标签。
func validHostname(h string) bool {
	if len(h) < 4 || len(h) > 253 {
		return false
	}
	if !containsDot(h) {
		return false
	}
	alpha := false
	for _, c := range h {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			alpha = true
		case c >= '0' && c <= '9', c == '-', c == '.':
		default:
			return false
		}
	}
	return alpha
}

func containsDot(s string) bool {
	for _, c := range s {
		if c == '.' {
			return true
		}
	}
	return false
}

func sameTuple(sp, dp uint16, fsp, fdp uint16) bool {
	return (sp == fsp && dp == fdp) || (sp == fdp && dp == fsp)
}

// l4Payload 返回 TCP/UDP 载荷。
func l4Payload(pkt []byte) []byte {
	if len(pkt) < 20 {
		return nil
	}
	switch pkt[0] >> 4 {
	case 4:
		ihl := int(pkt[0]&0x0f) * 4
		if len(pkt) < ihl+20 {
			return nil
		}
		if pkt[9] == 6 {
			doff := int(pkt[ihl+12]>>4) * 4
			if doff < 20 || len(pkt) < ihl+doff {
				return nil
			}
			if len(pkt) == ihl+doff {
				return nil
			}
			return pkt[ihl+doff:]
		}
		if pkt[9] == 17 && len(pkt) > ihl+8 {
			return pkt[ihl+8:]
		}
	case 6:
		if len(pkt) < 60 {
			return nil
		}
		if pkt[6] == 6 {
			doff := int(pkt[52]>>4) * 4
			if doff < 20 || len(pkt) < 40+doff {
				return nil
			}
			if len(pkt) == 40+doff {
				return nil
			}
			return pkt[40+doff:]
		}
		if pkt[6] == 17 && len(pkt) > 48 {
			return pkt[48:]
		}
	}
	return nil
}

func flowPorts(pkt []byte) (uint16, uint16) {
	if len(pkt) < 20 {
		return 0, 0
	}
	switch pkt[0] >> 4 {
	case 4:
		ihl := int(pkt[0]&0x0f) * 4
		if len(pkt) < ihl+4 {
			return 0, 0
		}
		return be16(pkt[ihl : ihl+2]), be16(pkt[ihl+2 : ihl+4])
	case 6:
		if len(pkt) < 44 {
			return 0, 0
		}
		return be16(pkt[40:42]), be16(pkt[42:44])
	}
	return 0, 0
}

func be16(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }
