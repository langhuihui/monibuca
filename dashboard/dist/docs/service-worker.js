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
    "revision": "75925fb89348802ecd737cdad2d5d801"
  },
  {
    "url": "assets/css/styles.d51c815b.css",
    "revision": "4a6b650244e5b709f84a81ad0565b485"
  },
  {
    "url": "assets/img/search.83621669.svg",
    "revision": "83621669651b9a3d4bf64d1a670ad856"
  },
  {
    "url": "assets/js/1.6babbc1d.js",
    "revision": "0142763a4e8630af56b66f5fa8320c5c"
  },
  {
    "url": "assets/js/2.190ec46a.js",
    "revision": "8dceb01ded85f36cc30e0e18371fe5d4"
  },
  {
    "url": "assets/js/3.7646e76c.js",
    "revision": "378c41587710afd31466293c83a6c738"
  },
  {
    "url": "assets/js/4.08fbc0d9.js",
    "revision": "0ea387538f5b25ef46237e3dcc1c1694"
  },
  {
    "url": "assets/js/app.d51c815b.js",
    "revision": "467373b055c3575c2082a1b9ab1769fd"
  },
  {
    "url": "develop.html",
    "revision": "92b13eb27581e4dc7008cf4205e5c215"
  },
  {
    "url": "history.html",
    "revision": "ef202ac3bf63e1bdef479a05d49d5a8d"
  },
  {
    "url": "index.html",
    "revision": "306f1a0a8ceadb0c85dae2342f0bd637"
  },
  {
    "url": "plugins.html",
    "revision": "ac2ce38679fb75f05e8e501f35d161bd"
  }
].concat(self.__precacheManifest || []);
workbox.precaching.suppressWarnings();
workbox.precaching.precacheAndRoute(self.__precacheManifest, {});
