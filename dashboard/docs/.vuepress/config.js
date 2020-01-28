module.exports = {
    dest: 'public/docs',
    serviceWorker: true,
    themeConfig: {
        sidebar: [
            ['/', '起步'],
            ['/develop', '插件开发'],
            [ '/history', '更新日志' ],
            {
                title: '内置插件',
                path: '/plugins/',
                children: [
                    '/plugins/jessica'
                ]
            },
        ]
    },
    title: 'Monibuca',
    base: '/docs/'
}