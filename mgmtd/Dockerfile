# mgmtd/Dockerfile — OpenWrt 容器集成测试用
#
# 构建：
#   docker build -t mgmtd:test .
# 运行：
#   docker run --rm -p 8080:8080 -e MGM_SESSION_SECRET=0123456789abcdef0123456789abcdef mgmtd:test
#
# 默认使用 alpine（静态 Go 二进制），集成测试时可替换为基础镜像为 openwrtorg/rootfs。

FROM alpine:3.20

# 二进制由 make build 产出；构建镜像前请先 `make build`
COPY bin/mgmtd /usr/sbin/mgmtd

ENV MGM_ADDR=:8080
ENV MGM_SESSION_SECRET=

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/v1/health || exit 1

ENTRYPOINT ["/usr/sbin/mgmtd"]
