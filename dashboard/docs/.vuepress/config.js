module.exports = {
    dest: 'public/docs',
    serviceWorker: true,
    themeConfig: {
        sidebar: [
            ['/', '起步'],
            ['/develop', '插件开发'],
            ['/history', '更新日志'],
            ['/plugins', '内置插件'],
            ['/design', '设计原理']
        ]
    },
    title: 'Monibuca',
    base: '/docs/'
}