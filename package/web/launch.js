(() => {
    'use strict';
    const status = document.getElementById('status');
    // DSM's URL launcher opens this same-origin page with the desktop as opener.
    // The CGI must receive DSM's CSRF token in addition to the login cookie.
    let token;
    try {
        token = window.opener.SYNO.SDS.Session.SynoToken;
    } catch (_) {
        // Missing/closed/cross-origin opener: never bypass DSM authentication.
    }
    if (typeof token !== 'string' || !token) {
        status.textContent = '请先登录 DSM，再从主菜单点击 DDNSTO 打开设置。直接输入 CGI 地址会因缺少 DSM 会话校验参数而被拒绝。';
        return;
    }
    const target = new URL('/webman/3rdparty/ddnsto/settings.cgi', location.origin);
    target.searchParams.set('SynoToken', token);
    location.replace(target.href);
})();
