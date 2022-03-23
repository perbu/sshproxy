#!/usr/bin/env bash

echo =============
env
echo =============

PRIVKEY="/mnt/admin"
PUBKEY="/mnt/admin.rsa.pub"
echo Starting opensshd on port 3222
/usr/sbin/sshd -D -e -p 3222
