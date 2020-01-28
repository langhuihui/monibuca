/**
 * Welcome to your Workbox-powered service worker!
 *
 * You'll need to register this file in your web app and you should
 * disable HTTP caching for this file too.
 * See https://goo.gl/nhQhGp
 *
 * The rest of the code is auto-generated. Please don't update this file
 * directly; instead, make changes to your Workbox build configuration
 * and re-run your build process.
 * See https://goo.gl/2aRDsh
 */

importScripts("https://storage.googleapis.com/workbox-cdn/releases/3.6.3/workbox-sw.js");

/**
 * The workboxSW.precacheAndRoute() method efficiently caches and responds to
 * requests for URLs in the manifest.
 * See https://goo.gl/S9QRab
 */
self.__precacheManifest = [
  {
    "url": "404.html",
    "revision": "0e4ff0fd403c5d29a13752bf3ef14d6d"
  },
  {
    "url": "assets/css/styles.92893aed.css",
    "revision": "07fa9a1fb782ef296585900714fac621"
  },
  {
    "url": "assets/img/search.83621669.svg",
    "revision": "83621669651b9a3d4bf64d1a670ad856"
  },
  {
    "url": "assets/js/1.685ed1cc.js",
    "revision": "97247c4d4a60db87b22488bfdf99197d"
  },
  {
    "url": "assets/js/2.a6d3efaf.js",
    "revision": "dca15d8c2b94dadcdce4386e7d628716"
  },
  {
    "url": "assets/js/3.a9fbea98.js",
    "revision": "7f6bca508f94f8f508a61cd05b582084"
  },
  {
    "url": "assets/js/4.727e40e9.js",
    "revision": "b8fca87e9c559c1fe3fa03b76d15c3bd"
  },
  {
    "url": "assets/js/5.78b155e8.js",
    "revision": "73d1f3053737ad68ee4ec4fa395fcae9"
  },
  {
    "url": "assets/js/6.35a311c9.js",
    "revision": "8bd2ad3294cf6a29e7326ca860d43250"
  },
  {
    "url": "assets/js/7.ab3a52c1.js",
    "revision": "437c1f4739f884dcff81135f7bc8450c"
  },
  {
    "url": "assets/js/app.92893aed.js",
    "revision": "6962a63ee86d8a65c9ae46100427d812"
  },
  {
    "url": "config.html",
    "revision": "73b0335d99419df53f57913cd7303509"
  },
  {
    "url": "develop.html",
    "revision": "bc1b3cad3f88b61fee351464cffc7d0f"
  },
  {
    "url": "history.html",
    "revision": "dc6b1088d3cc72f1f3a402c56a441a57"
  },
  {
    "url": "index.html",
    "revision": "577c237555d953d8a22d045212f0a92e"
  },
  {
    "url": "install.html",
    "revision": "d06af57877e4695ae1a81528e2f128f1"
  },
  {
    "url": "plugins/index.html",
    "revision": "e578381fed44a65589053470ea33795d"
  },
  {
    "url": "plugins/jessica.html",
    "revision": "fe406bf63d2d349727c63d6045fd2efa"
  }
].concat(self.__precacheManifest || []);
workbox.precaching.suppressWarnings();
workbox.precaching.precacheAndRoute(self.__precacheManifest, {});
