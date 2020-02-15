<template>
    <Layout class="layout">
        <Header style=" background:unset;text-align: center;">Monibuca 实例管理器</Header>
        <Content class="content">
            <Tabs value="name1">
                <TabPane label="实例" name="name1">
                    <InstanceList></InstanceList>
                </TabPane>
                <TabPane label="创建" name="name2">
                    <Steps :current="createStep">
                        <Step title="选择目录" content="选择创建实例的目录"></Step>
                        <Step title="选插件" content="选择要启用的插件"></Step>
                        <Step title="完成" content="完成实例创建"></Step>
                    </Steps>
                    <div style="margin:50px;width:auto">
                        <i-input v-model="createPath" v-if="createStep==0">
                            <Button slot="prepend" icon="md-arrow-round-up" @click="goUp"></Button>
                        </i-input>
                        <List v-else-if="createStep==1" border>
                            <ListItem v-for="(item,name) in plugins" :key="name">
                                <ListItemMeta :title="name" :description="item.Path"></ListItemMeta>
                                {{item.Config}}
                                <template slot="action">
                                    <li @click="removePlugin(name)">
                                        <Icon type="ios-trash"/>
                                        移除
                                    </li>
                                </template>
                            </ListItem>
                        </List>
                        <div v-else>
                            <h3>实例名称：</h3>
                            <i-input
                                    v-model="instanceName"
                                    :placeholder="createPath.split('/').pop()"
                            ></i-input>
                            <h4>安装路径：</h4>
                            <div>
                                <pre>{{createPath}}</pre>
                            </div>
                            <h4>启用的插件：</h4>
                            <div>
                                <pre>{{pluginStr}}</pre>
                            </div>
                            <h4>配置文件：</h4>
                            <div>
                                <pre>{{configStr}}</pre>
                            </div>
                        </div>
                        <ButtonGroup style="display:table;margin:50px auto;">
                            <Button
                                    size="large"
                                    type="primary"
                                    @click="createStep--"
                                    v-if="createStep!=0"
                            >
                                <Icon type="ios-arrow-back"></Icon>
                                上一步
                            </Button>
                            <Button
                                    size="large"
                                    type="success"
                                    @click="showAddPlugin=true"
                                    v-if="createStep==1"
                            >
                                +
                                添加插件
                            </Button>
                            <Button
                                    size="large"
                                    type="primary"
                                    @click="createStep++"
                                    v-if="createStep!=2"
                            >
                                下一步
                                <Icon type="ios-arrow-forward"></Icon>
                            </Button>
                            <Button size="large" type="success" @click="createInstance" v-else>开始创建</Button>
                        </ButtonGroup>
                    </div>
                </TabPane>
                <TabPane label="导入" name="name3">
                    <ImportInstance></ImportInstance>
                </TabPane>
            </Tabs>
        </Content>
        <Modal v-model="showAddPlugin" title="添加Plugin" @on-ok="addPlugin">
            <Form :model="formPlugin" label-position="top">
                <FormItem label="插件名称">
                    <i-input v-model="formPlugin.Name" placeholder="插件名称必须和插件注册时的名称一致"></i-input>
                </FormItem>
                <FormItem label="插件包地址">
                    <i-input v-model="formPlugin.Path">
                        <Button slot="append" @click="showBuiltinPlugin=true">内置插件</Button>
                    </i-input>
                </FormItem>
                <Alert  show-icon
                        type="warning"
                        v-if="!isBuiltInPlugin(formPlugin.Path)"
                >
                    如果该插件是私有仓库，请到服务器上输入：echo "machine {{privateHost}} login 用户名 password 密码" >> ~/.netrc
                    并且添加环境变量GOPRIVATE={{privateHost}}
                </Alert>
                <FormItem label="插件配置信息">
                    <i-input type="textarea" v-model="formPlugin.Config" placeholder="请输入toml格式"></i-input>
                </FormItem>
            </Form>
        </Modal>
        <Modal v-model="showBuiltinPlugin">
            <List>
                <ListItem v-for="(item,name) in $store.state.defaultPlugins" :key="name">
                    <ListItemMeta :title="name" :description="item[2]"></ListItemMeta>
                    <template slot="action">
                        <li @click="addBuiltin(name,item)">
                            <Icon type="ios-add"/>
                            添加
                        </li>
                    </template>
                </ListItem>
            </List>
        </Modal>
        <CreateInstance v-model="showCreate" :info="createInfo"></CreateInstance>
    </Layout>
</template>

<script>
    import CreateInstance from "../components/CreateInstance";
    import InstanceList from "../components/InstanceList";
    import ImportInstance from "../components/ImportInstance";

    export default {
        components: {
            CreateInstance,InstanceList,ImportInstance
        },
        data() {
            return {
                instanceName: "",
                createStep: 0,
                showCreate: false,
                createInfo: null,
                createPath: "/opt/monibuca",
                plugins: {},
                showAddPlugin: false,
                formPlugin: {},
                showBuiltinPlugin: false,
            };
        },
        computed: {
            pluginStr() {
                return Object.values(this.plugins)
                    .map(x => x.Path)
                    .join("\n");
            },
            configStr() {
                return Object.values(this.plugins)
                    .map(
                        x => `[Plugins.${x.Name}]
${x.Config || ""}`
                    )
                    .join("\n");
            },
            privateHost() {
                return (
                    (this.formPlugin.Path && this.formPlugin.Path.split("/")[0]) ||
                    "仓库域名"
                );
            }
        },

        methods: {
            isBuiltInPlugin(path){
                return Object.values(this.$store.state.defaultPlugins).some(x=>"github.com/langhuihui/monibuca/plugins/"+x[0]==path)
            },
            goUp() {
                let paths = this.createPath.split("/");
                paths.pop();
                this.createPath = paths.join("/");
            },
            createInstance() {
                this.showCreate = true;
                this.createInfo = {
                    Name: this.instanceName || this.createPath.split("/").pop(),
                    Path: this.createPath,
                    Plugins: Object.values(this.plugins).map(x => x.Path),
                    Config: this.configStr
                };
            },
            addPlugin() {
                this.plugins[this.formPlugin.Name] = this.formPlugin;
                this.formPlugin = {};
            },
            removePlugin(name) {
                delete this.plugins[name];
                this.$forceUpdate();
            },
            addBuiltin(name, item) {
                this.formPlugin.Name = name;
                this.formPlugin.Path = "github.com/langhuihui/monibuca/plugins/"+item[0];
                this.formPlugin.Config = item[1];
                this.showBuiltinPlugin = false;
            },
        }
    };
</script>

<style>
    .content {
        background: white;
    }

    pre {
        white-space: pre-wrap;
        word-wrap: break-word;
    }

    .ivu-tabs .ivu-tabs-tabpane {
        padding: 20px;
    }
</style>