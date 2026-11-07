# The build image plus what the browser suite drives: Node, Playwright's
# Chromium/Firefox/WebKit, and real Chrome (the bundled Chromium has no H.264).
ARG BUILD_IMAGE
FROM ${BUILD_IMAGE}

ARG NODE_VERSION
ARG NODE_SHA256
ARG PLAYWRIGHT_VERSION
ARG CHROME_VERSION
ARG CHROME_SHA256

RUN curl -fsSLo /tmp/node.txz "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" \
 && echo "${NODE_SHA256}  /tmp/node.txz" | sha256sum -c - \
 && tar -C /usr/local --strip-components=1 -xJf /tmp/node.txz && rm /tmp/node.txz

ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN npx -y "playwright@${PLAYWRIGHT_VERSION}" install --with-deps chromium firefox webkit \
 && curl -fsSLo /tmp/chrome.deb "https://dl.google.com/linux/chrome/deb/pool/main/g/google-chrome-stable/google-chrome-stable_${CHROME_VERSION}_amd64.deb" \
 && echo "${CHROME_SHA256}  /tmp/chrome.deb" | sha256sum -c - \
 && apt-get update && apt-get install -y --no-install-recommends /tmp/chrome.deb \
 && rm -rf /var/lib/apt/lists/* /tmp/chrome.deb /root/.npm \
 && chmod -R a+rX /ms-playwright
