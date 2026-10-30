/**
 * 导航结构直接对齐 iKuai 4.0 的分组骨架（从固件 www/json/menu.json 里取出来的
 * 13 个一级分组与顺序），这样从爱快迁移过来的人不用重新建立心智模型：
 * 「策略中心 → 分流策略」「对象管理 → IP分组」「虚拟专网 → PPTP/L2TP/OpenVPN」
 * 这些位置和爱快一致。
 *
 * 只挂已经真实存在的路由。iKuai 里有、我们还没实现的条目一律不挂：
 * 点进去一个空页面比没有入口更糟。新增页面时在对应分组里加一行即可。
 */

export type NavItem = {
  path: string;
  label: string;
  /** iKuai 里的 func_name，便于和固件脚本/menu.json 对照排查 */
  ik?: string;
};

export type NavGroup = {
  title: string;
  /** iKuai 一级分组的 func_name */
  ik?: string;
  items: NavItem[];
};

export const navGroups: NavGroup[] = [
  {
    title: '系统概览',
    ik: 'homepage',
    items: [{ path: '/', label: '系统概览', ik: 'homepage' }],
  },
  {
    title: '监控中心',
    ik: 'monitoring_center',
    items: [
      { path: '/stats', label: '负载监控', ik: 'monitor_system' },
      { path: '/traffic', label: '流量统计', ik: 'monitor_iface' },
      { path: '/dpi-flow', label: '行为洞察', ik: 'monitor_app_flow' },
      { path: '/clients', label: '终端监控', ik: 'monitor_lanip' },
      { path: '/online-users', label: '在线用户', ik: 'monitor_lanip' },
      { path: '/terminals', label: '终端列表', ik: 'monitor_lanip' },
      { path: '/linkcheck', label: '链路检测', ik: 'monitor_iface' },
    ],
  },
  {
    title: '策略中心',
    ik: 'security_center',
    items: [
      { path: '/security', label: 'ACL 规则', ik: 'acl' },
      { path: '/qos', label: '智能流控', ik: 'stream_control' },
      { path: '/quota', label: '终端限速', ik: 'terminal_qos' },
      { path: '/routing-policy', label: '分流策略', ik: 'fenliucelve' },
      { path: '/lb', label: '多线负载', ik: 'lb_pcc' },
      { path: '/l7', label: '应用协议控制', ik: 'acl_l7' },
      { path: '/dpi', label: '应用识别', ik: 'layer7' },
      { path: '/app-ident', label: '应用特征库', ik: 'app_lib' },
      { path: '/nac', label: 'MAC 访问控制', ik: 'acl_mac' },
      { path: '/ips', label: '入侵防御', ik: 'ips' },
      { path: '/threat', label: '威胁情报', ik: 'ioc_overview' },
      { path: '/security-plus', label: '安全增强', ik: 'urlsec' },
      { path: '/domain-route', label: '域名分流', ik: 'stream_domain' },
      { path: '/behavior', label: '行为记录', ik: 'monitor_app_flow' },
      { path: '/l3-apps', label: '跨三层应用', ik: 'cross_l3' },
    ],
  },
  {
    title: '对象管理',
    ik: 'object_manage',
    items: [
      { path: '/groups', label: 'IP / MAC / 终端分组', ik: 'route_object_ip' },
      { path: '/accounts', label: '账号管理', ik: 'auth' },
      { path: '/users', label: '用户管理', ik: 'pppuser' },
      { path: '/cert', label: '证书管理', ik: 'cert' },
    ],
  },
  {
    title: '网络配置',
    ik: 'network_config',
    items: [
      { path: '/interfaces', label: '接口与区域', ik: 'iface_check' },
      { path: '/multi-wan', label: '多线路设置', ik: 'multiwan' },
      { path: '/static-route', label: '静态路由', ik: 'route_static' },
      { path: '/network-advanced', label: '路由高级', ik: 'route_adv' },
      { path: '/l2', label: 'VLAN 配置', ik: 'vlan' },
      { path: '/ipv6', label: 'IPv6', ik: 'ipv6' },
      { path: '/dhcp', label: 'DHCP 服务', ik: 'dhcpd' },
      { path: '/dns', label: 'DNS 服务', ik: 'dns' },
      { path: '/dns-multiline', label: '多线路 DNS', ik: 'dns' },
      { path: '/upnp', label: 'UPnP 设置', ik: 'upnpd' },
      { path: '/network', label: '端口映射 / DMZ', ik: 'dnat' },
      { path: '/network-tools', label: '网络工具', ik: 'tool' },
    ],
  },
  {
    title: '无线管理',
    ik: 'wireless_manage',
    items: [{ path: '/wireless', label: 'AP 管理', ik: 'ac_status' }],
  },
  {
    title: '虚拟专网',
    ik: 'vpn',
    items: [
      { path: '/vpn', label: 'WireGuard', ik: 'wireguard' },
      { path: '/vpn-servers', label: 'PPTP / L2TP / OpenVPN', ik: 'pptp_server' },
      { path: '/vpn-client', label: 'VPN 客户端', ik: 'vpn_client' },
    ],
  },
  {
    title: '接入管理',
    ik: 'access_manage',
    items: [
      { path: '/radius', label: '认证服务', ik: 'radius' },
      { path: '/pppoe', label: 'PPPoE 服务端', ik: 'pppoe_server' },
      { path: '/portal', label: '认证页面', ik: 'auth_portal' },
      { path: '/voucher', label: '认证券', ik: 'voucher' },
    ],
  },
  {
    title: '扩展服务',
    ik: 'extend_service',
    items: [
      { path: '/services', label: '服务状态', ik: 'service' },
      { path: '/ddns', label: '动态域名', ik: 'ddns' },
      { path: '/remote', label: '内网穿透', ik: 'frp' },
      { path: '/snmp', label: 'SNMP 服务', ik: 'snmpd' },
      { path: '/cloud', label: '云服务绑定', ik: 'cloud' },
      { path: '/files', label: '文件管理', ik: 'file_mgmt' },
      { path: '/file-services', label: '文件服务（FTP/Samba/HTTP）', ik: 'ftp_server' },
      { path: '/plugins', label: '应用商店', ik: 'plugin' },
      { path: '/vms', label: '虚拟机（KVM）', ik: 'vm' },
    ],
  },
  {
    title: '日志中心',
    ik: 'log_center',
    items: [
      { path: '/logs', label: '功能日志', ik: 'log' },
      { path: '/log-categories', label: '分类日志', ik: 'log_arp' },
      { path: '/audit', label: '操作审计', ik: 'syslog-webadmin' },
      { path: '/alerts', label: '告警信息', ik: 'notice' },
      { path: '/diag', label: '诊断日志', ik: 'diag' },
      { path: '/diagnostics', label: '连通性诊断', ik: 'diag' },
    ],
  },
  {
    title: '设备设置',
    ik: 'device_setting',
    items: [
      { path: '/storage', label: '磁盘管理', ik: 'disk_mgmt' },
      { path: '/sysctl', label: '高级设置（内核参数）', ik: 'ik_sysctl' },
      { path: '/firmware', label: '升级备份', ik: 'upgrade' },
      { path: '/config-sync', label: '配置同步', ik: 'config' },
    ],
  },
];
