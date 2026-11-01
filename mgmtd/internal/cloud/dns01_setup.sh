#!/usr/bin/env bash
# 在 hk142 上把 router-cloud 的证书从 TLS-ALPN-01 切到 Cloudflare DNS-01。
#
# 用法（在 hk142 上以 root 执行，token 不会写入 shell 历史）：
#   CF_Token=YOUR_TOKEN CF_Account_ID=YOUR_ACCOUNT_ID bash dns01_setup.sh
#
# 为什么 DNS-01：TLS-ALPN-01 要求 443 全程由 nginx stream 透传到 router-cloud，
# 续期期间 nginx 无法同时服务其他 SNI；DNS-01 完全不占 443，续期零中断。
# 前置：1.1ai.best 的 NS 必须指向 Cloudflare（已确认：cruz.ns.cloudflare.com）。
set -euo pipefail

: "${CF_Token:?必须设置 CF_Token 环境变量}"
DOMAIN="${DOMAIN:-1.1ai.best}"
EMAIL="${CF_Email:-admin@1.1ai.best}"
CERT_DIR="${CERT_DIR:-/var/lib/router-cloud/certs}"
RENEW_CRON="${RENEW_CRON:-/etc/cron.d/acme-router-cloud}"

log() { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*"; }
die() { printf '错误: %s\n' "$*" >&2; exit 1; }
[ "$(id -u)" = 0 ] || die "需要 root"

# ---------------------------------------------------------------- 1. 安装 acme.sh（如未装）
if [ ! -x /root/.acme.sh/acme.sh ]; then
  log "安装 acme.sh ..."
  curl -sS -L https://get.acme.sh | sh -s email="$EMAIL"
  [ -x /root/.acme.sh/acme.sh ] || die "acme.sh 安装失败"
fi
ACME=/root/.acme.sh/acme.sh
log "acme.sh 已就绪: $($ACME --version 2>&1 | head -1)"

# ---------------------------------------------------------------- 2. DNS-01 签发
log "通过 Cloudflare DNS-01 签发 $DOMAIN ..."
export CF_Token
export CF_Account_ID="${CF_Account_ID:-}"
export CF_Zone_ID="${CF_Zone_ID:-}"
$ACME --issue --dns dns_cf -d "$DOMAIN" -d "www.$DOMAIN" --keylength 2048 --force 2>&1 | tail -5
CERT=""
KEY=""
for d in "$HOME/.acme.sh/$DOMAIN" "$HOME/.acme.sh/${DOMAIN}_ecc"; do
  if [ -f "$d/fullchain.cer" ] && [ -f "$d/$DOMAIN.key" ]; then
    CERT="$d/fullchain.cer"; KEY="$d/$DOMAIN.key"; break
  fi
done
[ -f "$CERT" ] && [ -f "$KEY" ] || die "签发失败，证书文件缺失"

# ---------------------------------------------------------------- 3. 安装证书到 router-cloud 的目录（acme.sh 续期后会自动再装）
install -d -m 0700 "$CERT_DIR"
$ACME --install-cert -d "$DOMAIN" \
  --fullchain-file "$CERT_DIR/$DOMAIN.crt" \
  --key-file "$CERT_DIR/$DOMAIN.key" \
  --reloadcmd "systemctl restart router-cloud" 2>&1 | tail -5
log "证书已装到 $CERT_DIR/$DOMAIN.{crt,key}"

# ---------------------------------------------------------------- 4. 切换 router-cloud 到文件证书模式
# 先备份环境文件
cp -a /etc/router-cloud/cloud.env /etc/router-cloud/cloud.env.bak.dns01.$(date +%Y%m%d_%H%M%S)
# 移除 ACME TLS-ALPN 相关变量（保留其他）
sed -i '/^ROUTER_CLOUD_ACME_HOST=/d; /^ROUTER_CLOUD_ACME_EMAIL=/d' /etc/router-cloud/cloud.env
# 追加证书路径（router-cloud 用 --tls-cert/--tls-key）
grep -q '^ROUTER_CLOUD_TLS_CERT=' /etc/router-cloud/cloud.env || echo "ROUTER_CLOUD_TLS_CERT=$CERT_DIR/$DOMAIN.crt" >> /etc/router-cloud/cloud.env
grep -q '^ROUTER_CLOUD_TLS_KEY=' /etc/router-cloud/cloud.env || echo "ROUTER_CLOUD_TLS_KEY=$CERT_DIR/$DOMAIN.key" >> /etc/router-cloud/cloud.env
log "环境文件已更新"

# ---------------------------------------------------------------- 5. 更新 systemd 单元（用文件证书启动）
UNIT=/etc/systemd/system/router-cloud.service
cp -a "$UNIT" "$UNIT.bak.dns01.$(date +%Y%m%d_%H%M%S)"
# 替换 ExecStart 行
python3 - "$UNIT" "$CERT_DIR" "$DOMAIN" <<'PY'
import re, sys
unit, certdir, domain = sys.argv[1], sys.argv[2], sys.argv[3]
s = open(unit).read()
s = re.sub(r'^ExecStart=.*$', f'ExecStart=/opt/router-cloud/router-cloud -addr 127.0.0.1:8444 -store /var/lib/router-cloud/state.json -tls-cert {certdir}/{domain}.crt -tls-key {certdir}/{domain}.key', s, flags=re.M)
open(unit, 'w').write(s)
PY
systemctl daemon-reload
systemctl restart router-cloud
sleep 2
systemctl is-active --quiet router-cloud || { journalctl -u router-cloud -n 20 --no-pager; die "router-cloud 启动失败"; }
log "router-cloud 已切换到 DNS-01 证书"

# ---------------------------------------------------------------- 6. 续期 cron（每天 03:00 检查）
cat > "$RENEW_CRON" <<EOF
SHELL=/bin/bash
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
0 3 * * * root $ACME --cron --home /root/.acme.sh >/var/log/acme-router-cloud.log 2>&1
EOF
chmod 0644 "$RENEW_CRON"
log "续期 cron 已写入 $RENEW_CRON"

# ---------------------------------------------------------------- 7. 自检
echo ""
log "自检："
curl -fsS -m 8 -o /dev/null -w "  https://$DOMAIN → HTTP %{http_code}\n" "https://$DOMAIN/"
openssl x509 -in "$CERT_DIR/$DOMAIN.crt" -noout -subject -issuer -dates 2>/dev/null | sed 's/^/  /'
echo ""
log "完成。今后证书续期由 acme.sh cron 自动完成，router-cloud 会通过 --reloadcmd 重启加载新证书。"
log "如需回滚到 TLS-ALPN：还原 /etc/router-cloud/cloud.env.bak.dns01.* 和 .service.bak.dns01.*，并重新启动。"
