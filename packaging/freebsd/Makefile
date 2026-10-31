# FreeBSD Port Makefile for SSG
# Install: cd /usr/ports/www/ssg && make install clean

PORTNAME=	ssg
DISTVERSION=	1.9.0
CATEGORIES=	www

MAINTAINER=	spagu@github.com
COMMENT=	Fast static site generator written in Go
WWW=		https://github.com/spagu/ssg

LICENSE=	BUSL-1.1
LICENSE_NAME=	Business Source License 1.1
LICENSE_FILE=	${WRKSRC}/LICENSE
LICENSE_PERMS=	dist-mirror dist-sell pkg-mirror pkg-sell auto-accept

USES=		go:modules

USE_GITHUB=	yes
GH_ACCOUNT=	spagu
GH_PROJECT=	ssg
GH_TAGNAME=	v${DISTVERSION}

GO_MODULE=	github.com/spagu/ssg
GO_TARGET=	./cmd/ssg
GO_BUILDFLAGS=	-ldflags "-s -w"

PLIST_FILES=	bin/ssg

.include <bsd.port.mk>
