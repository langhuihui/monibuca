import{G as x,S as nt,u as at,a2 as Mt,a9 as It,aI as Ft,_ as $,Y as Lt,aJ as rt,J as At,w as ot,aK as Y,a0 as Ct,aL as Z,aM as zt,aN as ut,P as st,aO as vt,aP as ht,a3 as Bt,k as ft,l as ct,h as K,ad as pt,aQ as gt,C as Q,s as mt,E as X,aE as Ht,aa as Ut,ah as Wt,ab as $t,aR as Jt,a5 as Zt,o as Nt,aS as qt,aT as dt,af as jt,aU as Kt,aV as Qt,aW as Xt,aX as Yt,F as xt,B as te,K as ee,aY as ae}from"./installCanvasRenderer-cb1ba546.js";function tt(n,e,t,a){return e&&!isNaN(e[0])&&!isNaN(e[1])&&!(a.isIgnore&&a.isIgnore(t))&&!(a.clipShape&&!a.clipShape.contain(e[0],e[1]))&&n.getItemVisual(t,"symbol")!=="none"}function yt(n){return n!=null&&!Ft(n)&&(n={isIgnore:n}),n||{}}function St(n){var e=n.hostModel,t=e.getModel("emphasis");return{emphasisItemStyle:t.getModel("itemStyle").getItemStyle(),blurItemStyle:e.getModel(["blur","itemStyle"]).getItemStyle(),selectItemStyle:e.getModel(["select","itemStyle"]).getItemStyle(),focus:t.get("focus"),blurScope:t.get("blurScope"),emphasisDisabled:t.get("disabled"),hoverScale:t.get("scale"),labelStatesModels:It(e),cursorStyle:e.get("cursor")}}var re=function(){function n(e){this.group=new x,this._SymbolCtor=e||nt}return n.prototype.updateData=function(e,t){this._progressiveEls=null,t=yt(t);var a=this.group,o=e.hostModel,r=this._data,i=this._SymbolCtor,s=t.disableAnimation,u=St(e),l={disableAnimation:s},f=t.getSymbolPoint||function(v){return e.getItemLayout(v)};r||a.removeAll(),e.diff(r).add(function(v){var c=f(v);if(tt(e,c,v,t)){var h=new i(e,v,u,l);h.setPosition(c),e.setItemGraphicEl(v,h),a.add(h)}}).update(function(v,c){var h=r.getItemGraphicEl(c),g=f(v);if(!tt(e,g,v,t)){a.remove(h);return}var d=e.getItemVisual(v,"symbol")||"circle",S=h&&h.getSymbolType&&h.getSymbolType();if(!h||S&&S!==d)a.remove(h),h=new i(e,v,u,l),h.setPosition(g);else{h.updateData(e,v,u,l);var p={x:g[0],y:g[1]};s?h.attr(p):at(h,p,o)}a.add(h),e.setItemGraphicEl(v,h)}).remove(function(v){var c=r.getItemGraphicEl(v);c&&c.fadeOut(function(){a.remove(c)},o)}).execute(),this._getSymbolPoint=f,this._data=e},n.prototype.updateLayout=function(){var e=this,t=this._data;t&&t.eachItemGraphicEl(function(a,o){var r=e._getSymbolPoint(o);a.setPosition(r),a.markRedraw()})},n.prototype.incrementalPrepareUpdate=function(e){this._seriesScope=St(e),this._data=null,this.group.removeAll()},n.prototype.incrementalUpdate=function(e,t,a){this._progressiveEls=[],a=yt(a);function o(u){u.isGroup||(u.incremental=!0,u.ensureState("emphasis").hoverLayer=!0)}for(var r=e.start;r<e.end;r++){var i=t.getItemLayout(r);if(tt(t,i,r,a)){var s=new this._SymbolCtor(t,r,this._seriesScope);s.traverse(o),s.setPosition(i),this.group.add(s),t.setItemGraphicEl(r,s),this._progressiveEls.push(s)}}},n.prototype.eachRendered=function(e){Mt(this._progressiveEls||this.group,e)},n.prototype.remove=function(e){var t=this.group,a=this._data;a&&e?a.eachItemGraphicEl(function(o){o.fadeOut(function(){t.remove(o)},a.hostModel)}):t.removeAll()},n}();const kt=re;var ie=function(n){$(e,n);function e(){var t=n!==null&&n.apply(this,arguments)||this;return t.type=e.type,t.hasSymbolVisual=!0,t}return e.prototype.getInitialData=function(t){return Lt(null,this,{useEncodeDefaulter:!0})},e.prototype.getLegendIcon=function(t){var a=new x,o=rt("line",0,t.itemHeight/2,t.itemWidth,0,t.lineStyle.stroke,!1);a.add(o),o.setStyle(t.lineStyle);var r=this.getData().getVisual("symbol"),i=this.getData().getVisual("symbolRotate"),s=r==="none"?"circle":r,u=t.itemHeight*.8,l=rt(s,(t.itemWidth-u)/2,(t.itemHeight-u)/2,u,u,t.itemStyle.fill);a.add(l),l.setStyle(t.itemStyle);var f=t.iconRotate==="inherit"?i:t.iconRotate||0;return l.rotation=f*Math.PI/180,l.setOrigin([t.itemWidth/2,t.itemHeight/2]),s.indexOf("empty")>-1&&(l.style.stroke=l.style.fill,l.style.fill="#fff",l.style.lineWidth=2),a},e.type="series.line",e.dependencies=["grid","polar"],e.defaultOption={z:3,coordinateSystem:"cartesian2d",legendHoverLink:!0,clip:!0,label:{position:"top"},endLabel:{show:!1,valueAnimation:!0,distance:8},lineStyle:{width:2,type:"solid"},emphasis:{scale:!0},step:!1,smooth:!1,smoothMonotone:null,symbol:"emptyCircle",symbolSize:4,symbolRotate:null,showSymbol:!0,showAllSymbol:"auto",connectNulls:!1,sampling:"none",animationEasing:"linear",progressive:0,hoverLayerThreshold:1/0,universalTransition:{divideShape:"clone"},triggerLineEvent:!1},e}(At);const ne=ie;function Tt(n,e,t){var a=n.getBaseAxis(),o=n.getOtherAxis(a),r=oe(o,t),i=a.dim,s=o.dim,u=e.mapDimension(s),l=e.mapDimension(i),f=s==="x"||s==="radius"?1:0,v=ot(n.dimensions,function(g){return e.mapDimension(g)}),c=!1,h=e.getCalculationInfo("stackResultDimension");return Y(e,v[0])&&(c=!0,v[0]=h),Y(e,v[1])&&(c=!0,v[1]=h),{dataDimsForPoint:v,valueStart:r,valueAxisDim:s,baseAxisDim:i,stacked:!!c,valueDim:u,baseDim:l,baseDataOffset:f,stackedOverDimension:e.getCalculationInfo("stackedOverDimension")}}function oe(n,e){var t=0,a=n.scale.getExtent();return e==="start"?t=a[0]:e==="end"?t=a[1]:Ct(e)&&!isNaN(e)?t=e:a[0]>0?t=a[0]:a[1]<0&&(t=a[1]),t}function Et(n,e,t,a){var o=NaN;n.stacked&&(o=t.get(t.getCalculationInfo("stackedOverDimension"),a)),isNaN(o)&&(o=n.valueStart);var r=n.baseDataOffset,i=[];return i[r]=t.get(n.baseDim,a),i[1-r]=o,e.dataToPoint(i)}function se(n,e){var t=[];return e.diff(n).add(function(a){t.push({cmd:"+",idx:a})}).update(function(a,o){t.push({cmd:"=",idx:o,idx1:a})}).remove(function(a){t.push({cmd:"-",idx:a})}).execute(),t}function le(n,e,t,a,o,r,i,s){for(var u=se(n,e),l=[],f=[],v=[],c=[],h=[],g=[],d=[],S=Tt(o,e,i),p=n.getLayout("points")||[],m=e.getLayout("points")||[],y=0;y<u.length;y++){var P=u[y],w=!0,b=void 0,_=void 0;switch(P.cmd){case"=":b=P.idx*2,_=P.idx1*2;var L=p[b],I=p[b+1],C=m[_],A=m[_+1];(isNaN(L)||isNaN(I))&&(L=C,I=A),l.push(L,I),f.push(C,A),v.push(t[b],t[b+1]),c.push(a[_],a[_+1]),d.push(e.getRawIndex(P.idx1));break;case"+":var D=P.idx,N=S.dataDimsForPoint,R=o.dataToPoint([e.get(N[0],D),e.get(N[1],D)]);_=D*2,l.push(R[0],R[1]),f.push(m[_],m[_+1]);var V=Et(S,o,e,D);v.push(V[0],V[1]),c.push(a[_],a[_+1]),d.push(e.getRawIndex(D));break;case"-":w=!1}w&&(h.push(P),g.push(g.length))}g.sort(function(j,E){return d[j]-d[E]});for(var O=l.length,F=Z(O),T=Z(O),k=Z(O),z=Z(O),B=[],y=0;y<g.length;y++){var q=g[y],G=y*2,M=q*2;F[G]=l[M],F[G+1]=l[M+1],T[G]=f[M],T[G+1]=f[M+1],k[G]=v[M],k[G+1]=v[M+1],z[G]=c[M],z[G+1]=c[M+1],B[y]=h[q]}return{current:F,next:T,stackedOnCurrent:k,stackedOnNext:z,status:B}}var H=Math.min,U=Math.max;function J(n,e){return isNaN(n)||isNaN(e)}function it(n,e,t,a,o,r,i,s,u){for(var l,f,v,c,h,g,d=t,S=0;S<a;S++){var p=e[d*2],m=e[d*2+1];if(d>=o||d<0)break;if(J(p,m)){if(u){d+=r;continue}break}if(d===t)n[r>0?"moveTo":"lineTo"](p,m),v=p,c=m;else{var y=p-l,P=m-f;if(y*y+P*P<.5){d+=r;continue}if(i>0){for(var w=d+r,b=e[w*2],_=e[w*2+1];b===p&&_===m&&S<a;)S++,w+=r,d+=r,b=e[w*2],_=e[w*2+1],p=e[d*2],m=e[d*2+1],y=p-l,P=m-f;var L=S+1;if(u)for(;J(b,_)&&L<a;)L++,w+=r,b=e[w*2],_=e[w*2+1];var I=.5,C=0,A=0,D=void 0,N=void 0;if(L>=a||J(b,_))h=p,g=m;else{C=b-l,A=_-f;var R=p-l,V=b-p,O=m-f,F=_-m,T=void 0,k=void 0;if(s==="x"){T=Math.abs(R),k=Math.abs(V);var z=C>0?1:-1;h=p-z*T*i,g=m,D=p+z*k*i,N=m}else if(s==="y"){T=Math.abs(O),k=Math.abs(F);var B=A>0?1:-1;h=p,g=m-B*T*i,D=p,N=m+B*k*i}else T=Math.sqrt(R*R+O*O),k=Math.sqrt(V*V+F*F),I=k/(k+T),h=p-C*i*(1-I),g=m-A*i*(1-I),D=p+C*i*I,N=m+A*i*I,D=H(D,U(b,p)),N=H(N,U(_,m)),D=U(D,H(b,p)),N=U(N,H(_,m)),C=D-p,A=N-m,h=p-C*T/k,g=m-A*T/k,h=H(h,U(l,p)),g=H(g,U(f,m)),h=U(h,H(l,p)),g=U(g,H(f,m)),C=p-h,A=m-g,D=p+C*k/T,N=m+A*k/T}n.bezierCurveTo(v,c,h,g,p,m),v=D,c=N}else n.lineTo(p,m)}l=p,f=m,d+=r}return S}var Ot=function(){function n(){this.smooth=0,this.smoothConstraint=!0}return n}(),ue=function(n){$(e,n);function e(t){var a=n.call(this,t)||this;return a.type="ec-polyline",a}return e.prototype.getDefaultStyle=function(){return{stroke:"#000",fill:null}},e.prototype.getDefaultShape=function(){return new Ot},e.prototype.buildPath=function(t,a){var o=a.points,r=0,i=o.length/2;if(a.connectNulls){for(;i>0&&J(o[i*2-2],o[i*2-1]);i--);for(;r<i&&J(o[r*2],o[r*2+1]);r++);}for(;r<i;)r+=it(t,o,r,i,i,1,a.smooth,a.smoothMonotone,a.connectNulls)+1},e.prototype.getPointOn=function(t,a){this.path||(this.createPathProxy(),this.buildPath(this.path,this.shape));for(var o=this.path,r=o.data,i=zt.CMD,s,u,l=a==="x",f=[],v=0;v<r.length;){var c=r[v++],h=void 0,g=void 0,d=void 0,S=void 0,p=void 0,m=void 0,y=void 0;switch(c){case i.M:s=r[v++],u=r[v++];break;case i.L:if(h=r[v++],g=r[v++],y=l?(t-s)/(h-s):(t-u)/(g-u),y<=1&&y>=0){var P=l?(g-u)*y+u:(h-s)*y+s;return l?[t,P]:[P,t]}s=h,u=g;break;case i.C:h=r[v++],g=r[v++],d=r[v++],S=r[v++],p=r[v++],m=r[v++];var w=l?ut(s,h,d,p,t,f):ut(u,g,S,m,t,f);if(w>0)for(var b=0;b<w;b++){var _=f[b];if(_<=1&&_>=0){var P=l?vt(u,g,S,m,_):vt(s,h,d,p,_);return l?[t,P]:[P,t]}}s=p,u=m;break}}},e}(st),ve=function(n){$(e,n);function e(){return n!==null&&n.apply(this,arguments)||this}return e}(Ot),he=function(n){$(e,n);function e(t){var a=n.call(this,t)||this;return a.type="ec-polygon",a}return e.prototype.getDefaultShape=function(){return new ve},e.prototype.buildPath=function(t,a){var o=a.points,r=a.stackedOnPoints,i=0,s=o.length/2,u=a.smoothMonotone;if(a.connectNulls){for(;s>0&&J(o[s*2-2],o[s*2-1]);s--);for(;i<s&&J(o[i*2],o[i*2+1]);i++);}for(;i<s;){var l=it(t,o,i,s,s,1,a.smooth,u,a.connectNulls);it(t,r,i+l-1,l,s,-1,a.stackedOnSmooth,u,a.connectNulls),i+=l+1,t.closePath()}},e}(st);function bt(n,e){if(n.length===e.length){for(var t=0;t<n.length;t++)if(n[t]!==e[t])return;return!0}}function _t(n){for(var e=1/0,t=1/0,a=-1/0,o=-1/0,r=0;r<n.length;){var i=n[r++],s=n[r++];isNaN(i)||(e=Math.min(i,e),a=Math.max(i,a)),isNaN(s)||(t=Math.min(s,t),o=Math.max(s,o))}return[[e,t],[a,o]]}function Dt(n,e){var t=_t(n),a=t[0],o=t[1],r=_t(e),i=r[0],s=r[1];return Math.max(Math.abs(a[0]-i[0]),Math.abs(a[1]-i[1]),Math.abs(o[0]-s[0]),Math.abs(o[1]-s[1]))}function Pt(n){return Ct(n)?n:n?.5:0}function fe(n,e,t){if(!t.valueDim)return[];for(var a=e.count(),o=Z(a*2),r=0;r<a;r++){var i=Et(t,n,e,r);o[r*2]=i[0],o[r*2+1]=i[1]}return o}function W(n,e,t,a){var o=e.getBaseAxis(),r=o.dim==="x"||o.dim==="radius"?0:1,i=[],s=0,u=[],l=[],f=[],v=[];if(a){for(s=0;s<n.length;s+=2)!isNaN(n[s])&&!isNaN(n[s+1])&&v.push(n[s],n[s+1]);n=v}for(s=0;s<n.length-2;s+=2)switch(f[0]=n[s+2],f[1]=n[s+3],l[0]=n[s],l[1]=n[s+1],i.push(l[0],l[1]),t){case"end":u[r]=f[r],u[1-r]=l[1-r],i.push(u[0],u[1]);break;case"middle":var c=(l[r]+f[r])/2,h=[];u[r]=h[r]=c,u[1-r]=l[1-r],h[1-r]=f[1-r],i.push(u[0],u[1]),i.push(h[0],h[1]);break;default:u[r]=l[r],u[1-r]=f[1-r],i.push(u[0],u[1])}return i.push(n[s++],n[s++]),i}function ce(n,e){var t=[],a=n.length,o,r;function i(f,v,c){var h=f.coord,g=(c-h)/(v.coord-h),d=Xt(g,[f.color,v.color]);return{coord:c,color:d}}for(var s=0;s<a;s++){var u=n[s],l=u.coord;if(l<0)o=u;else if(l>e){r?t.push(i(r,u,e)):o&&t.push(i(o,u,0),i(o,u,e));break}else o&&(t.push(i(o,u,0)),o=null),t.push(u),r=u}return t}function pe(n,e,t){var a=n.getVisual("visualMeta");if(!(!a||!a.length||!n.count())&&e.type==="cartesian2d"){for(var o,r,i=a.length-1;i>=0;i--){var s=n.getDimensionInfo(a[i].dimension);if(o=s&&s.coordDim,o==="x"||o==="y"){r=a[i];break}}if(r){var u=e.getAxis(o),l=ot(r.stops,function(y){return{coord:u.toGlobalCoord(u.dataToCoord(y.value)),color:y.color}}),f=l.length,v=r.outerColors.slice();f&&l[0].coord>l[f-1].coord&&(l.reverse(),v.reverse());var c=ce(l,o==="x"?t.getWidth():t.getHeight()),h=c.length;if(!h&&f)return l[0].coord<0?v[1]?v[1]:l[f-1].color:v[0]?v[0]:l[0].color;var g=10,d=c[0].coord-g,S=c[h-1].coord+g,p=S-d;if(p<.001)return"transparent";Nt(c,function(y){y.offset=(y.coord-d)/p}),c.push({offset:h?c[h-1].offset:.5,color:v[1]||"transparent"}),c.unshift({offset:h?c[0].offset:.5,color:v[0]||"transparent"});var m=new qt(0,0,0,0,c,!0);return m[o]=d,m[o+"2"]=S,m}}}function ge(n,e,t){var a=n.get("showAllSymbol"),o=a==="auto";if(!(a&&!o)){var r=t.getAxesByScale("ordinal")[0];if(r&&!(o&&me(r,e))){var i=e.mapDimension(r.dim),s={};return Nt(r.getViewLabels(),function(u){var l=r.scale.getRawOrdinalNumber(u.tickValue);s[l]=1}),function(u){return!s.hasOwnProperty(e.get(i,u))}}}}function me(n,e){var t=n.getExtent(),a=Math.abs(t[1]-t[0])/n.scale.count();isNaN(a)&&(a=0);for(var o=e.count(),r=Math.max(1,Math.round(o/5)),i=0;i<o;i+=r)if(nt.getSymbolSize(e,i)[n.isHorizontal()?1:0]*1.5>a)return!1;return!0}function de(n,e){return isNaN(n)||isNaN(e)}function ye(n){for(var e=n.length/2;e>0&&de(n[e*2-2],n[e*2-1]);e--);return e-1}function wt(n,e){return[n[e*2],n[e*2+1]]}function Se(n,e,t){for(var a=n.length/2,o=t==="x"?0:1,r,i,s=0,u=-1,l=0;l<a;l++)if(i=n[l*2+o],!(isNaN(i)||isNaN(n[l*2+1-o]))){if(l===0){r=i;continue}if(r<=e&&i>=e||r>=e&&i<=e){u=l;break}s=l,r=i}return{range:[s,u],t:(e-r)/(i-r)}}function Rt(n){if(n.get(["endLabel","show"]))return!0;for(var e=0;e<dt.length;e++)if(n.get([dt[e],"endLabel","show"]))return!0;return!1}function et(n,e,t,a){if(jt(e,"cartesian2d")){var o=a.getModel("endLabel"),r=o.get("valueAnimation"),i=a.getData(),s={lastFrameIndex:0},u=Rt(a)?function(h,g){n._endLabelOnDuring(h,g,i,s,r,o,e)}:null,l=e.getBaseAxis().isHorizontal(),f=Kt(e,t,a,function(){var h=n._endLabel;h&&t&&s.originalX!=null&&h.attr({x:s.originalX,y:s.originalY})},u);if(!a.get("clip",!0)){var v=f.shape,c=Math.max(v.width,v.height);l?(v.y-=c,v.height+=c*2):(v.x-=c,v.width+=c*2)}return u&&u(1,f),f}else return Qt(e,t,a)}function be(n,e){var t=e.getBaseAxis(),a=t.isHorizontal(),o=t.inverse,r=a?o?"right":"left":"center",i=a?"middle":o?"top":"bottom";return{normal:{align:n.get("align")||r,verticalAlign:n.get("verticalAlign")||i}}}var _e=function(n){$(e,n);function e(){return n!==null&&n.apply(this,arguments)||this}return e.prototype.init=function(){var t=new x,a=new kt;this.group.add(a.group),this._symbolDraw=a,this._lineGroup=t},e.prototype.render=function(t,a,o){var r=this,i=t.coordinateSystem,s=this.group,u=t.getData(),l=t.getModel("lineStyle"),f=t.getModel("areaStyle"),v=u.getLayout("points")||[],c=i.type==="polar",h=this._coordSys,g=this._symbolDraw,d=this._polyline,S=this._polygon,p=this._lineGroup,m=!a.ssr&&t.isAnimationEnabled(),y=!f.isEmpty(),P=f.get("origin"),w=Tt(i,u,P),b=y&&fe(i,u,w),_=t.get("showSymbol"),L=t.get("connectNulls"),I=_&&!c&&ge(t,u,i),C=this._data;C&&C.eachItemGraphicEl(function(E,Gt){E.__temp&&(s.remove(E),C.setItemGraphicEl(Gt,null))}),_||g.remove(),s.add(p);var A=c?!1:t.get("step"),D;i&&i.getArea&&t.get("clip",!0)&&(D=i.getArea(),D.width!=null?(D.x-=.1,D.y-=.1,D.width+=.2,D.height+=.2):D.r0&&(D.r0-=.5,D.r+=.5)),this._clipShapeForSymbol=D;var N=pe(u,i,o)||u.getVisual("style")[u.getVisual("drawType")];if(!(d&&h.type===i.type&&A===this._step))_&&g.updateData(u,{isIgnore:I,clipShape:D,disableAnimation:!0,getSymbolPoint:function(E){return[v[E*2],v[E*2+1]]}}),m&&this._initSymbolLabelAnimation(u,i,D),A&&(v=W(v,i,A,L),b&&(b=W(b,i,A,L))),d=this._newPolyline(v),y?S=this._newPolygon(v,b):S&&(p.remove(S),S=this._polygon=null),c||this._initOrUpdateEndLabel(t,i,ht(N)),p.setClipPath(et(this,i,!0,t));else{y&&!S?S=this._newPolygon(v,b):S&&!y&&(p.remove(S),S=this._polygon=null),c||this._initOrUpdateEndLabel(t,i,ht(N));var R=p.getClipPath();if(R){var V=et(this,i,!1,t);Bt(R,{shape:V.shape},t)}else p.setClipPath(et(this,i,!0,t));_&&g.updateData(u,{isIgnore:I,clipShape:D,disableAnimation:!0,getSymbolPoint:function(E){return[v[E*2],v[E*2+1]]}}),(!bt(this._stackedOnPoints,b)||!bt(this._points,v))&&(m?this._doUpdateAnimation(u,b,i,o,A,P,L):(A&&(v=W(v,i,A,L),b&&(b=W(b,i,A,L))),d.setShape({points:v}),S&&S.setShape({points:v,stackedOnPoints:b})))}var O=t.getModel("emphasis"),F=O.get("focus"),T=O.get("blurScope"),k=O.get("disabled");if(d.useStyle(ft(l.getLineStyle(),{fill:"none",stroke:N,lineJoin:"bevel"})),ct(d,t,"lineStyle"),d.style.lineWidth>0&&t.get(["emphasis","lineStyle","width"])==="bolder"){var z=d.getState("emphasis").style;z.lineWidth=+d.style.lineWidth+1}K(d).seriesIndex=t.seriesIndex,pt(d,F,T,k);var B=Pt(t.get("smooth")),q=t.get("smoothMonotone");if(d.setShape({smooth:B,smoothMonotone:q,connectNulls:L}),S){var G=u.getCalculationInfo("stackedOnSeries"),M=0;S.useStyle(ft(f.getAreaStyle(),{fill:N,opacity:.7,lineJoin:"bevel",decal:u.getVisual("style").decal})),G&&(M=Pt(G.get("smooth"))),S.setShape({smooth:B,stackedOnSmooth:M,smoothMonotone:q,connectNulls:L}),ct(S,t,"areaStyle"),K(S).seriesIndex=t.seriesIndex,pt(S,F,T,k)}var j=function(E){r._changePolyState(E)};u.eachItemGraphicEl(function(E){E&&(E.onHoverStateChange=j)}),this._polyline.onHoverStateChange=j,this._data=u,this._coordSys=i,this._stackedOnPoints=b,this._points=v,this._step=A,this._valueOrigin=P,t.get("triggerLineEvent")&&(this.packEventData(t,d),S&&this.packEventData(t,S))},e.prototype.packEventData=function(t,a){K(a).eventData={componentType:"series",componentSubType:"line",componentIndex:t.componentIndex,seriesIndex:t.seriesIndex,seriesName:t.name,seriesType:"line"}},e.prototype.highlight=function(t,a,o,r){var i=t.getData(),s=gt(i,r);if(this._changePolyState("emphasis"),!(s instanceof Array)&&s!=null&&s>=0){var u=i.getLayout("points"),l=i.getItemGraphicEl(s);if(!l){var f=u[s*2],v=u[s*2+1];if(isNaN(f)||isNaN(v)||this._clipShapeForSymbol&&!this._clipShapeForSymbol.contain(f,v))return;var c=t.get("zlevel")||0,h=t.get("z")||0;l=new nt(i,s),l.x=f,l.y=v,l.setZ(c,h);var g=l.getSymbolPath().getTextContent();g&&(g.zlevel=c,g.z=h,g.z2=this._polyline.z2+1),l.__temp=!0,i.setItemGraphicEl(s,l),l.stopSymbolAnimation(!0),this.group.add(l)}l.highlight()}else Q.prototype.highlight.call(this,t,a,o,r)},e.prototype.downplay=function(t,a,o,r){var i=t.getData(),s=gt(i,r);if(this._changePolyState("normal"),s!=null&&s>=0){var u=i.getItemGraphicEl(s);u&&(u.__temp?(i.setItemGraphicEl(s,null),this.group.remove(u)):u.downplay())}else Q.prototype.downplay.call(this,t,a,o,r)},e.prototype._changePolyState=function(t){var a=this._polygon;mt(this._polyline,t),a&&mt(a,t)},e.prototype._newPolyline=function(t){var a=this._polyline;return a&&this._lineGroup.remove(a),a=new ue({shape:{points:t},segmentIgnoreThreshold:2,z2:10}),this._lineGroup.add(a),this._polyline=a,a},e.prototype._newPolygon=function(t,a){var o=this._polygon;return o&&this._lineGroup.remove(o),o=new he({shape:{points:t,stackedOnPoints:a},segmentIgnoreThreshold:2}),this._lineGroup.add(o),this._polygon=o,o},e.prototype._initSymbolLabelAnimation=function(t,a,o){var r,i,s=a.getBaseAxis(),u=s.inverse;a.type==="cartesian2d"?(r=s.isHorizontal(),i=!1):a.type==="polar"&&(r=s.dim==="angle",i=!0);var l=t.hostModel,f=l.get("animationDuration");X(f)&&(f=f(null));var v=l.get("animationDelay")||0,c=X(v)?v(null):v;t.eachItemGraphicEl(function(h,g){var d=h;if(d){var S=[h.x,h.y],p=void 0,m=void 0,y=void 0;if(o)if(i){var P=o,w=a.pointToCoord(S);r?(p=P.startAngle,m=P.endAngle,y=-w[1]/180*Math.PI):(p=P.r0,m=P.r,y=w[0])}else{var b=o;r?(p=b.x,m=b.x+b.width,y=h.x):(p=b.y+b.height,m=b.y,y=h.y)}var _=m===p?0:(y-p)/(m-p);u&&(_=1-_);var L=X(v)?v(g):f*_+c,I=d.getSymbolPath(),C=I.getTextContent();d.attr({scaleX:0,scaleY:0}),d.animateTo({scaleX:1,scaleY:1},{duration:200,setToFinal:!0,delay:L}),C&&C.animateFrom({style:{opacity:0}},{duration:300,delay:L}),I.disableLabelAnimation=!0}})},e.prototype._initOrUpdateEndLabel=function(t,a,o){var r=t.getModel("endLabel");if(Rt(t)){var i=t.getData(),s=this._polyline,u=i.getLayout("points");if(!u){s.removeTextContent(),this._endLabel=null;return}var l=this._endLabel;l||(l=this._endLabel=new Ht({z2:200}),l.ignoreClip=!0,s.setTextContent(this._endLabel),s.disableLabelAnimation=!0);var f=ye(u);f>=0&&(Ut(s,It(t,"endLabel"),{inheritColor:o,labelFetcher:t,labelDataIndex:f,defaultText:function(v,c,h){return h!=null?Wt(i,h):$t(i,v)},enableTextSetter:!0},be(r,a)),s.textConfig.position=null)}else this._endLabel&&(this._polyline.removeTextContent(),this._endLabel=null)},e.prototype._endLabelOnDuring=function(t,a,o,r,i,s,u){var l=this._endLabel,f=this._polyline;if(l){t<1&&r.originalX==null&&(r.originalX=l.x,r.originalY=l.y);var v=o.getLayout("points"),c=o.hostModel,h=c.get("connectNulls"),g=s.get("precision"),d=s.get("distance")||0,S=u.getBaseAxis(),p=S.isHorizontal(),m=S.inverse,y=a.shape,P=m?p?y.x:y.y+y.height:p?y.x+y.width:y.y,w=(p?d:0)*(m?-1:1),b=(p?0:-d)*(m?-1:1),_=p?"x":"y",L=Se(v,P,_),I=L.range,C=I[1]-I[0],A=void 0;if(C>=1){if(C>1&&!h){var D=wt(v,I[0]);l.attr({x:D[0]+w,y:D[1]+b}),i&&(A=c.getRawValue(I[0]))}else{var D=f.getPointOn(P,_);D&&l.attr({x:D[0]+w,y:D[1]+b});var N=c.getRawValue(I[0]),R=c.getRawValue(I[1]);i&&(A=Jt(o,g,N,R,L.t))}r.lastFrameIndex=I[0]}else{var V=t===1||r.lastFrameIndex>0?I[0]:0,D=wt(v,V);i&&(A=c.getRawValue(V)),l.attr({x:D[0]+w,y:D[1]+b})}i&&Zt(l).setLabelText(A)}},e.prototype._doUpdateAnimation=function(t,a,o,r,i,s,u){var l=this._polyline,f=this._polygon,v=t.hostModel,c=le(this._data,t,this._stackedOnPoints,a,this._coordSys,o,this._valueOrigin),h=c.current,g=c.stackedOnCurrent,d=c.next,S=c.stackedOnNext;if(i&&(h=W(c.current,o,i,u),g=W(c.stackedOnCurrent,o,i,u),d=W(c.next,o,i,u),S=W(c.stackedOnNext,o,i,u)),Dt(h,d)>3e3||f&&Dt(g,S)>3e3){l.stopAnimation(),l.setShape({points:d}),f&&(f.stopAnimation(),f.setShape({points:d,stackedOnPoints:S}));return}l.shape.__points=c.current,l.shape.points=h;var p={shape:{points:d}};c.current!==h&&(p.shape.__points=c.next),l.stopAnimation(),at(l,p,v),f&&(f.setShape({points:h,stackedOnPoints:g}),f.stopAnimation(),at(f,{shape:{stackedOnPoints:S}},v),l.shape.points!==f.shape.points&&(f.shape.points=l.shape.points));for(var m=[],y=c.status,P=0;P<y.length;P++){var w=y[P].cmd;if(w==="="){var b=t.getItemGraphicEl(y[P].idx1);b&&m.push({el:b,ptIdx:P})}}l.animators&&l.animators.length&&l.animators[0].during(function(){f&&f.dirtyShape();for(var _=l.shape.__points,L=0;L<m.length;L++){var I=m[L].el,C=m[L].ptIdx*2;I.x=_[C],I.y=_[C+1],I.markRedraw()}})},e.prototype.remove=function(t){var a=this.group,o=this._data;this._lineGroup.removeAll(),this._symbolDraw.remove(!0),o&&o.eachItemGraphicEl(function(r,i){r.__temp&&(a.remove(r),o.setItemGraphicEl(i,null))}),this._polyline=this._polygon=this._coordSys=this._points=this._stackedOnPoints=this._endLabel=this._data=null},e.type="line",e}(Q);const De=_e;function lt(n,e){return{seriesType:n,plan:Yt(),reset:function(t){var a=t.getData(),o=t.coordinateSystem,r=t.pipelineContext,i=e||r.large;if(o){var s=ot(o.dimensions,function(h){return a.mapDimension(h)}).slice(0,2),u=s.length,l=a.getCalculationInfo("stackResultDimension");Y(a,s[0])&&(s[0]=l),Y(a,s[1])&&(s[1]=l);var f=a.getStore(),v=a.getDimensionIndex(s[0]),c=a.getDimensionIndex(s[1]);return u&&{progress:function(h,g){for(var d=h.end-h.start,S=i&&Z(d*u),p=[],m=[],y=h.start,P=0;y<h.end;y++){var w=void 0;if(u===1){var b=f.get(v,y);w=o.dataToPoint(b,null,m)}else p[0]=f.get(v,y),p[1]=f.get(c,y),w=o.dataToPoint(p,null,m);i?(S[P++]=w[0],S[P++]=w[1]):g.setItemLayout(y,w.slice())}i&&g.setLayout("points",S)}}}}}}var Pe={average:function(n){for(var e=0,t=0,a=0;a<n.length;a++)isNaN(n[a])||(e+=n[a],t++);return t===0?NaN:e/t},sum:function(n){for(var e=0,t=0;t<n.length;t++)e+=n[t]||0;return e},max:function(n){for(var e=-1/0,t=0;t<n.length;t++)n[t]>e&&(e=n[t]);return isFinite(e)?e:NaN},min:function(n){for(var e=1/0,t=0;t<n.length;t++)n[t]<e&&(e=n[t]);return isFinite(e)?e:NaN},nearest:function(n){return n[0]}},we=function(n){return Math.round(n.length/2)};function Ie(n){return{seriesType:n,reset:function(e,t,a){var o=e.getData(),r=e.get("sampling"),i=e.coordinateSystem,s=o.count();if(s>10&&i.type==="cartesian2d"&&r){var u=i.getBaseAxis(),l=i.getOtherAxis(u),f=u.getExtent(),v=a.getDevicePixelRatio(),c=Math.abs(f[1]-f[0])*(v||1),h=Math.round(s/c);if(isFinite(h)&&h>1){r==="lttb"&&e.setData(o.lttbDownSample(o.mapDimension(l.dim),1/h));var g=void 0;xt(r)?g=Pe[r]:X(r)&&(g=r),g&&e.setData(o.downSample(o.mapDimension(l.dim),1/h,g,we))}}}}}function Ve(n){n.registerChartView(De),n.registerSeriesModel(ne),n.registerLayout(lt("line",!0)),n.registerVisual({seriesType:"line",reset:function(e){var t=e.getData(),a=e.getModel("lineStyle").getLineStyle();a&&!a.stroke&&(a.stroke=t.getVisual("style").fill),t.setVisual("legendLineStyle",a)}}),n.registerProcessor(n.PRIORITY.PROCESSOR.STATISTIC,Ie("line"))}var Le=function(n){$(e,n);function e(){var t=n!==null&&n.apply(this,arguments)||this;return t.type=e.type,t.hasSymbolVisual=!0,t}return e.prototype.getInitialData=function(t,a){return Lt(null,this,{useEncodeDefaulter:!0})},e.prototype.getProgressive=function(){var t=this.option.progressive;return t??(this.option.large?5e3:this.get("progressive"))},e.prototype.getProgressiveThreshold=function(){var t=this.option.progressiveThreshold;return t??(this.option.large?1e4:this.get("progressiveThreshold"))},e.prototype.brushSelector=function(t,a,o){return o.point(a.getItemLayout(t))},e.prototype.getZLevelKey=function(){return this.getData().count()>this.getProgressiveThreshold()?this.id:""},e.type="series.scatter",e.dependencies=["grid","polar","geo","singleAxis","calendar"],e.defaultOption={coordinateSystem:"cartesian2d",z:2,legendHoverLink:!0,symbolSize:10,large:!1,largeThreshold:2e3,itemStyle:{opacity:.8},emphasis:{scale:!0},clip:!0,select:{itemStyle:{borderColor:"#212121"}},universalTransition:{divideShape:"clone"}},e}(At);const Ae=Le;var Vt=4,Ce=function(){function n(){}return n}(),Ne=function(n){$(e,n);function e(t){var a=n.call(this,t)||this;return a._off=0,a.hoverDataIdx=-1,a}return e.prototype.getDefaultShape=function(){return new Ce},e.prototype.reset=function(){this.notClear=!1,this._off=0},e.prototype.buildPath=function(t,a){var o=a.points,r=a.size,i=this.symbolProxy,s=i.shape,u=t.getContext?t.getContext():t,l=u&&r[0]<Vt,f=this.softClipShape,v;if(l){this._ctx=u;return}for(this._ctx=null,v=this._off;v<o.length;){var c=o[v++],h=o[v++];isNaN(c)||isNaN(h)||f&&!f.contain(c,h)||(s.x=c-r[0]/2,s.y=h-r[1]/2,s.width=r[0],s.height=r[1],i.buildPath(t,s,!0))}this.incremental&&(this._off=v,this.notClear=!0)},e.prototype.afterBrush=function(){var t=this.shape,a=t.points,o=t.size,r=this._ctx,i=this.softClipShape,s;if(r){for(s=this._off;s<a.length;){var u=a[s++],l=a[s++];isNaN(u)||isNaN(l)||i&&!i.contain(u,l)||r.fillRect(u-o[0]/2,l-o[1]/2,o[0],o[1])}this.incremental&&(this._off=s,this.notClear=!0)}},e.prototype.findDataIndex=function(t,a){for(var o=this.shape,r=o.points,i=o.size,s=Math.max(i[0],4),u=Math.max(i[1],4),l=r.length/2-1;l>=0;l--){var f=l*2,v=r[f]-s/2,c=r[f+1]-u/2;if(t>=v&&a>=c&&t<=v+s&&a<=c+u)return l}return-1},e.prototype.contain=function(t,a){var o=this.transformCoordToLocal(t,a),r=this.getBoundingRect();if(t=o[0],a=o[1],r.contain(t,a)){var i=this.hoverDataIdx=this.findDataIndex(t,a);return i>=0}return this.hoverDataIdx=-1,!1},e.prototype.getBoundingRect=function(){var t=this._rect;if(!t){for(var a=this.shape,o=a.points,r=a.size,i=r[0],s=r[1],u=1/0,l=1/0,f=-1/0,v=-1/0,c=0;c<o.length;){var h=o[c++],g=o[c++];u=Math.min(h,u),f=Math.max(h,f),l=Math.min(g,l),v=Math.max(g,v)}t=this._rect=new te(u-i/2,l-s/2,f-u+i,v-l+s)}return t},e}(st),ke=function(){function n(){this.group=new x}return n.prototype.updateData=function(e,t){this._clear();var a=this._create();a.setShape({points:e.getLayout("points")}),this._setCommon(a,e,t)},n.prototype.updateLayout=function(e){var t=e.getLayout("points");this.group.eachChild(function(a){if(a.startIndex!=null){var o=(a.endIndex-a.startIndex)*2,r=a.startIndex*4*2;t=new Float32Array(t.buffer,r,o)}a.setShape("points",t),a.reset()})},n.prototype.incrementalPrepareUpdate=function(e){this._clear()},n.prototype.incrementalUpdate=function(e,t,a){var o=this._newAdded[0],r=t.getLayout("points"),i=o&&o.shape.points;if(i&&i.length<2e4){var s=i.length,u=new Float32Array(s+r.length);u.set(i),u.set(r,s),o.endIndex=e.end,o.setShape({points:u})}else{this._newAdded=[];var l=this._create();l.startIndex=e.start,l.endIndex=e.end,l.incremental=!0,l.setShape({points:r}),this._setCommon(l,t,a)}},n.prototype.eachRendered=function(e){this._newAdded[0]&&e(this._newAdded[0])},n.prototype._create=function(){var e=new Ne({cursor:"default"});return e.ignoreCoarsePointer=!0,this.group.add(e),this._newAdded.push(e),e},n.prototype._setCommon=function(e,t,a){var o=t.hostModel;a=a||{};var r=t.getVisual("symbolSize");e.setShape("size",r instanceof Array?r:[r,r]),e.softClipShape=a.clipShape||null,e.symbolProxy=rt(t.getVisual("symbol"),0,0,0,0),e.setColor=e.symbolProxy.setColor;var i=e.shape.size[0]<Vt;e.useStyle(o.getModel("itemStyle").getItemStyle(i?["color","shadowBlur","shadowColor"]:["color"]));var s=t.getVisual("style"),u=s&&s.fill;u&&e.setColor(u);var l=K(e);l.seriesIndex=o.seriesIndex,e.on("mousemove",function(f){l.dataIndex=null;var v=e.hoverDataIdx;v>=0&&(l.dataIndex=v+(e.startIndex||0))})},n.prototype.remove=function(){this._clear()},n.prototype._clear=function(){this._newAdded=[],this.group.removeAll()},n}();const Te=ke;var Ee=function(n){$(e,n);function e(){var t=n!==null&&n.apply(this,arguments)||this;return t.type=e.type,t}return e.prototype.render=function(t,a,o){var r=t.getData(),i=this._updateSymbolDraw(r,t);i.updateData(r,{clipShape:this._getClipShape(t)}),this._finished=!0},e.prototype.incrementalPrepareRender=function(t,a,o){var r=t.getData(),i=this._updateSymbolDraw(r,t);i.incrementalPrepareUpdate(r),this._finished=!1},e.prototype.incrementalRender=function(t,a,o){this._symbolDraw.incrementalUpdate(t,a.getData(),{clipShape:this._getClipShape(a)}),this._finished=t.end===a.getData().count()},e.prototype.updateTransform=function(t,a,o){var r=t.getData();if(this.group.dirty(),!this._finished||r.count()>1e4)return{update:!0};var i=lt("").reset(t,a,o);i.progress&&i.progress({start:0,end:r.count(),count:r.count()},r),this._symbolDraw.updateLayout(r)},e.prototype.eachRendered=function(t){this._symbolDraw&&this._symbolDraw.eachRendered(t)},e.prototype._getClipShape=function(t){var a=t.coordinateSystem,o=a&&a.getArea&&a.getArea();return t.get("clip",!0)?o:null},e.prototype._updateSymbolDraw=function(t,a){var o=this._symbolDraw,r=a.pipelineContext,i=r.large;return(!o||i!==this._isLargeDraw)&&(o&&o.remove(),o=this._symbolDraw=i?new Te:new kt,this._isLargeDraw=i,this.group.removeAll()),this.group.add(o.group),o},e.prototype.remove=function(t,a){this._symbolDraw&&this._symbolDraw.remove(!0),this._symbolDraw=null},e.prototype.dispose=function(){},e.type="scatter",e}(Q);const Oe=Ee;function Ge(n){ee(ae),n.registerSeriesModel(Ae),n.registerChartView(Oe),n.registerLayout(lt("scatter"))}export{Ve as a,Ie as d,Ge as i};