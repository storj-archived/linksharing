ARG DOCKER_ARCH
FROM ${DOCKER_ARCH:-amd64}/alpine
ARG TAG
ARG GOARCH
ENV GOARCH ${GOARCH}
EXPOSE 7777
WORKDIR /app
VOLUME /root/.local/share/storj/linksharing
COPY release/${TAG}/linksharing_linux_${GOARCH:-amd64} /app/linksharing
COPY web/ /app/web/
COPY entrypoint /entrypoint
ENTRYPOINT ["/entrypoint"]
ENV STORJ_CONFIG_DIR=/root/.local/share/storj/linksharing
ENV STORJ_SERVER_ADDRESS=0.0.0.0:7777
