import{s,_ as F,a as h}from"./index-c14b60e2.js";import{m as w,l as E}from"./vendor-ec30964e.js";function f(u){return s({url:"/api/wx/isSubscribe",method:"POST",data:u})}function S(){return s({url:"/api/wx/config",method:"GET"})}function B(u){return s({url:"/api/wx/qrcode",method:"POST",data:u})}const r=u=>(Vue.pushScopeId("data-v-ffa39570"),u=u(),Vue.popScopeId(),u),I={class:"view-account"},x=r(()=>Vue.createElementVNode("div",{class:"view-account-header"},null,-1)),y={class:"view-account-container"},C={class:"view-account-top"},N={class:"view-account-top-logo"},k={class:"view-account-top-desc"},b={class:"erweima"},A=r(()=>Vue.createElementVNode("div",null,"微信扫码关注官方微信公众号【不卡科技】即可快速登录",-1)),P=["src"],T=Vue.defineComponent({setup(u){const i=naive.useMessage(),n=Vue.ref(""),c=Vue.ref(""),d=w(),l=E(),m=e=>new Promise(o=>{const t=setTimeout(()=>{clearTimeout(t),o()},e)}),p=()=>new Promise((e,o)=>{f({myId:sessionStorage.getItem("myId")}).then(t=>{e(t)})}),a=async()=>{var e;try{const o=await p();if(console.log("是否关注",o),o&&o.data){i.success("登录成功");const t=decodeURIComponent(((e=l.query)==null?void 0:e.redirect)||"/");d.replace(t)}else await m(5e3),a()}catch(o){throw"报错"+o}},_=()=>new Promise(async e=>{const o=await S();console.log("token信息对象11: ",o);const t=o,v=["openLocation","updateTimelineShareData","chooseImage","getLocation","onMenuShareQZone","getNetworkType"];wx.config({debug:!0,appId:t.appId,timestamp:t.timestamp,nonceStr:t.nonceStr,signature:t.signature,jsApiList:v}),e()}),V=()=>{let e=sessionStorage.getItem("myId");e||(e=parseInt(Math.random()*1e6),e<1&&(e=666666)),sessionStorage.setItem("myId",e),console.log("开始获取--"),B({expire_seconds:604800,action_name:"QR_SCENE",action_info:{scene:{scene_id:e}}}).then(t=>{c.value=t.ticket,n.value=`https://mp.weixin.qq.com/cgi-bin/showqrcode?ticket=${c.value}`})};Vue.onMounted(async()=>{await _(),V(),a()});const g=h();return(e,o)=>{const t=Vue.resolveComponent("svg-icon");return Vue.openBlock(),Vue.createElementBlock("div",I,[x,Vue.createElementVNode("div",y,[Vue.createElementVNode("div",C,[Vue.createElementVNode("div",N,[Vue.createVNode(t,{name:"logo",width:"256px"})]),Vue.createElementVNode("div",k," 实例管理平台"+Vue.toDisplayString(Vue.unref(g).isSaas?"（在线版）":"（私有化体验版）"),1)]),Vue.createElementVNode("div",b,[A,n.value?(Vue.openBlock(),Vue.createElementBlock("img",{key:0,src:n.value,class:"iframe-box"},null,8,P)):Vue.createCommentVNode("",!0)])])])}}}),D=F(T,[["__scopeId","data-v-ffa39570"]]);export{D as default};