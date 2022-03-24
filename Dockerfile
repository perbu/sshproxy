
# docker run --cap-add NET_ADMIN --mount type=bind,source=/Users/perbu/git/celerway/n2-rssh/testing/keys,target=/mnt --rm  -it --network host sshd

FROM alpine
COPY server.sh /
RUN apk add bash curl openssh-server rsync
RUN adduser -D celerway && passwd -u celerway
RUN mkdir -p /home/celerway/.ssh/ && cd /home/celerway/ && chown -R celerway .ssh && chmod 0700 .ssh && chown -R celerway /home/celerway
COPY sshd_config /etc/ssh/
COPY id_rsa /etc/ssh/id_rsa
COPY id_rsa.pub /etc/ssh/id_rsa.pub

CMD /server.sh
EXPOSE 3222
