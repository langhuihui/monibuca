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
    "revision": "d38782b915b3a279cefd140f22641acd"
  },
  {
    "url": "assets/css/styles.3074e6c5.css",
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
    "url": "assets/js/4.8c104e50.js",
    "revision": "fcf87307bc87f10fd810a997fbf1e922"
  },
  {
    "url": "assets/js/5.bd73b45e.js",
    "revision": "9e29fc1b0c76fdeee1a9b6073856ca62"
  },
  {
    "url": "assets/js/app.3074e6c5.js",
    "revision": "b6a945f2750302d570f029cbac574f31"
  },
  {
    "url": "design.html",
    "revision": "628808833c989cb15f8c4f2fb15acf35"
  },
  {
    "url": "develop.html",
    "revision": "ea562dc02f652b15d39ebbf76fcee218"
  },
  {
    "url": "history.html",
    "revision": "89d4a6b5baa198d4ac42539f3324dc26"
  },
  {
    "url": "index.html",
    "revision": "a27f94a0d3aa4b464d6a680dcb01cbe3"
  },
  {
    "url": "plugins.html",
    "revision": "a6ada6e01fabe8a493f52759596561b8"
  }
].concat(self.__precacheManifest || []);
workbox.precaching.suppressWarnings();
workbox.precaching.precacheAndRoute(self.__precacheManifest, {});
