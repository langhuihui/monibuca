import Vue from 'vue'
import VueRouter from 'vue-router'
import Instances from "../views/Instances"
Vue.use(VueRouter)

const routes = [
  {
    path: '/',
    name: 'instances',
    component: Instances
  }
]

const router = new VueRouter({
  mode: 'history',
  base: process.env.BASE_URL,
  routes
})

export default router
