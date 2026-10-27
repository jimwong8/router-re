include $(TOPDIR)/rules.mk

PKG_NAME:=router-re
PKG_VERSION:=0.1.0
PKG_RELEASE:=1

PKG_MAINTAINER:=jimwong <jimwong@example.com>
PKG_LICENSE:=MIT

# Go build env
GO_PKG:=github.com/jimwong/router-re/mgmtd
GOOS:=linux
GOARCH:=$(subst aarch64,aarch64,$(ARCH))
# OpenWrt ARCH mapping -> Go GOARCH
ifeq ($(ARCH),x86_64)
  GOARCH:=amd64
endif
ifeq ($(ARCH),aarch64)
  GOARCH:=arm64
endif
ifeq ($(ARCH),arm)
  GOARCH:=arm
endif
ifeq ($(ARCH),mips)
  GOARCH:=mips
endif
ifeq ($(ARCH),mipsel)
  GOARCH:=mipsle
endif

include $(INCLUDE_DIR)/package.mk

define Package/router-re
  SECTION:=net
  CATEGORY:=Network
  TITLE:=router-re configuration management daemon
  URL:=https://github.com/jimwong/router-re
  # ip-full: BusyBox 的 ip 不支持 `-j`（JSON）和 `-br`（brief），
  #   而 mgmtd 的路由/邻居/VLAN 回读全部依赖这两个选项。实测在 24.10.8
  #   的 BusyBox v1.36.1 上 `ip -j route` 直接打印 usage，接口返回空。
  # nftables: 防火墙/ACL/NAT 下发。
  # conntrack-tools: 在线用户断开（conntrack -D -s <ip>）。
  # kmod-nf-conntrack-netlink: 上面那条命令需要的内核模块。
  DEPENDS:=+ip-full +nftables +conntrack-tools +kmod-nf-conntrack-netlink
endef

define Package/router-re/description
 router-re (mgmtd) is a Go-based configuration management daemon for routers,
 providing REST API for auth, firewall, NAT, DHCP, QoS, WAN policy routing,
 dynamic routing (FRR), VPN (WireGuard) and system info.
endef

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
	cp -r /home/jimwong/projects/router-re/mgmtd $(PKG_BUILD_DIR)/mgmtd
	cp -r /home/jimwong/projects/router-re/web $(PKG_BUILD_DIR)/web 2>/dev/null || true
endef

define Build/Compile
	# Cross-compile the Go binary using the SDK toolchain env.
	cd $(PKG_BUILD_DIR)/mgmtd && \
		CGO_ENABLED=0 \
		GOOS=$(GOOS) GOARCH=$(GOARCH) \
		GOCACHE=$(BUILD_DIR)/.gocache \
		go build -trimpath -ldflags="-s -w" -o $(PKG_BUILD_DIR)/mgmtd_bin ./cmd/mgmtd
endef

define Package/router-re/install
	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/mgmtd_bin $(1)/usr/sbin/mgmtd
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) $(CURDIR)/files/mgmtd.init $(1)/etc/init.d/mgmtd
	$(INSTALL_DIR) $(1)/etc/config
	echo 'config router-re "main"' > $(1)/etc/config/router-re
	echo '	option addr ":8080"' >> $(1)/etc/config/router-re
	# session_secret intentionally absent: init script generates a random
	# 32-byte key on first boot via /dev/urandom and persists to UCI.
endef

$(eval $(call BuildPackage,router-re))
