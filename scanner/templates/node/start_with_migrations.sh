#!/bin/sh -ex

npx prisma migrate deploy

{{ .packager }} run start
