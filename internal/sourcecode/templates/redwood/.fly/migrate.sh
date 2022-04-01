#!/bin/sh

set -ex

yarn rw prisma migrate deploy
