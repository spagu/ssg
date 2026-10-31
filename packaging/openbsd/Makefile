# OpenBSD Port Makefile for SSG

COMMENT=	Fast static site generator written in Go

V=		1.9.0
DISTNAME=	ssg-${V}

CATEGORIES=	www

HOMEPAGE=	https://github.com/spagu/ssg

MAINTAINER=	spagu@github.com

# Business Source License 1.1 (source available; production use free with the
# "Built with ssg" notice, BSD-3-Clause four years after each release)
PERMIT_PACKAGE=	Yes

WANTLIB=	c pthread

MASTER_SITES=	https://github.com/spagu/ssg/archive/refs/tags/
DISTFILES=	v${V}.tar.gz

MODULES=	lang/go

MODGO_TYPE=	bin
MODGO_FLAGS=	-ldflags="-s -w"

ALL_TARGET=	github.com/spagu/ssg/cmd/ssg

do-install:
	${INSTALL_PROGRAM} ${WRKBUILD}/bin/ssg ${PREFIX}/bin/

.include <bsd.port.mk>
