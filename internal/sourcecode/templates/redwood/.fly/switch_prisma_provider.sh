#!/bin/sh

mv api/db/schema.prisma api/db/schema.prisma.dist
sed s/sqlite/postgresql/ api/db/schema.prisma.dist > api/db/schema.prisma
