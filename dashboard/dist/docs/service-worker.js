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
    "revision": "a3d9e915fd09958cab2da343fcad58b0"
  },
  {
    "url": "assets/css/styles.ad3166d6.css",
    "revision": "4a6b650244e5b709f84a81ad0565b485"
  },
  {
    "url": "assets/img/search.83621669.svg",
    "revision": "83621669651b9a3d4bf64d1a670ad856"
  },
  {
    "url": "assets/js/1.83ce04b6.js",
    "revision": "3cabebb5c79c8280aee24ac6e4545650"
  },
  {
    "url": "assets/js/2.142d04d2.js",
    "revision": "027f4f643a10a465692035fe692cd94f"
  },
  {
    "url": "assets/js/3.2b6c987b.js",
    "revision": "27a988ab518e3f04db65045269d55841"
  },
  {
    "url": "assets/js/4.ad90d74a.js",
    "revision": "dcbfc54b67e9e6e33dbb2a303e842bdc"
  },
  {
    "url": "assets/js/5.36121818.js",
    "revision": "550520f0f388530dfbc4cc40a3dee264"
  },
  {
    "url": "assets/js/app.ad3166d6.js",
    "revision": "fdeb68e4b7b08c2c7bdcaf7462f34b16"
  },
  {
    "url": "develop.html",
    "revision": "f7461297bdc3ce303d517f0fc6dedd78"
  },
  {
    "url": "history.html",
    "revision": "c4ba2b13246f7a9e0039852fe381de6e"
  },
  {
    "url": "index.html",
    "revision": "3371a6066d0c2229a521d0659f25b174"
  },
  {
    "url": "plugins/index.html",
    "revision": "511b5cef98368f5f0ab35e667766e367"
  },
  {
    "url": "plugins/jessica.html",
    "revision": "65ca2965d5d514441d64b4d5e38cd696"
  }
].concat(self.__precacheManifest || []);
workbox.precaching.suppressWarnings();
workbox.precaching.precacheAndRoute(self.__precacheManifest, {});
