<template>
    <div id="app">
        <div>Monibuca</div>
        <Menu mode="horizontal" :active-name="selectedMenu" style="position: absolute;top: 0;right: 0;">
            <MenuItem name="home" to="/">首页</MenuItem>
            <MenuItem name="docs" to="/docs" target="_blank">
                文档
            </MenuItem>
            <MenuItem name="console" to="console">
                控制台
            </MenuItem>
            <Submenu name="plugins">
                <template slot="title">
                    内置插件
                </template>
                <MenuGroup title="发布者/订阅者">
                    <MenuItem name="cluster">集群</MenuItem>
                    <MenuItem name="rtmp">RTMP</MenuItem>
                </MenuGroup>
                <MenuGroup title="订阅者">
                    <MenuItem name="jessica">Jessica</MenuItem>
                    <MenuItem name="HDL">Http-Flv</MenuItem>
                    <MenuItem name="record">录制Flv</MenuItem>
                </MenuGroup>
                <MenuGroup title="发布者">
                    <MenuItem name="HLS">HLS</MenuItem>
                    <MenuItem name="TS">TS</MenuItem>
                </MenuGroup>
                <MenuGroup title="钩子">
                    <MenuItem name="Auth">验证</MenuItem>
                    <MenuItem name="QoS">QoS</MenuItem>
                    <MenuItem name="gateway">网关</MenuItem>
                </MenuGroup>
            </Submenu>
            <MenuItem name="4" to="about">
                支持
            </MenuItem>
        </Menu>
        <router-view class="content"></router-view>
        <div>Copyright © 2019-2020 dexter 苏ICP备20001212号</div>
    </div>
</template>

<script>

    export default {
        name: 'app',
        data(){
            return {
                selectedMenu:"home"
            }
        }
    }
</script>

<style>
    body,html{
        height: 100%;
    }
    #app {
        font-family: 'Avenir', Helvetica, Arial, sans-serif;
        -webkit-font-smoothing: antialiased;
        -moz-osx-font-smoothing: grayscale;
        text-align: center;
        color: #184c18;
        position: relative;
        height: 100%;
    }


    #app > div:first-child {
        position: absolute;
        top: 10px;
        left: 30px;
        font-size: x-large;
    }
    .content{
        padding-top: 60px;
    }
</style>
