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
    "revision": "deb4e5a58824b45354e458242cfc5f68"
  },
  {
    "url": "assets/css/styles.1fc3f87c.css",
    "revision": "4a6b650244e5b709f84a81ad0565b485"
  },
  {
    "url": "assets/img/search.83621669.svg",
    "revision": "83621669651b9a3d4bf64d1a670ad856"
  },
  {
    "url": "assets/js/1.05f88c5b.js",
    "revision": "afa91e5980d9ef9c164df65dfcc212f3"
  },
  {
    "url": "assets/js/2.69b20946.js",
    "revision": "e49c74ff572f38586aa8d2d0295f6d67"
  },
  {
    "url": "assets/js/3.197b5253.js",
    "revision": "68528a936ba6abcb203ebf1e3b779f7b"
  },
  {
    "url": "assets/js/4.2a48a234.js",
    "revision": "84b7fc95074e5673dc9cbac5e43ac54b"
  },
  {
    "url": "assets/js/5.bd73b45e.js",
    "revision": "9e29fc1b0c76fdeee1a9b6073856ca62"
  },
  {
    "url": "assets/js/app.1fc3f87c.js",
    "revision": "3433613b6d3d61b3b5b1472588698b88"
  },
  {
    "url": "design.html",
    "revision": "d3141cee964d685ecdd1115018ebb5a5"
  },
  {
    "url": "develop.html",
    "revision": "41466536941a8fefa23255ec78f400e2"
  },
  {
    "url": "history.html",
    "revision": "a30a702553aeb8c3654a7b51a26591de"
  },
  {
    "url": "index.html",
    "revision": "e1a828d24054341e2439ac0a59d1f673"
  },
  {
    "url": "plugins.html",
    "revision": "20cdd78205e1bd15b6f5f0ad1b203ab0"
  }
].concat(self.__precacheManifest || []);
workbox.precaching.suppressWarnings();
workbox.precaching.precacheAndRoute(self.__precacheManifest, {});
