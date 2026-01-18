# FreeBSD Port Makefile for SSG
# Install: cd /usr/ports/www/ssg && make install clean

PORTNAME=	ssg
DISTVERSION=	1.4.1
CATEGORIES=	www

MAINTAINER=	spagu@github.com
COMMENT=	Fast static site generator written in Go
WWW=		https://github.com/spagu/ssg

LICENSE=	BSD3CLAUSE
LICENSE_FILE=	${WRKSRC}/LICENSE

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
