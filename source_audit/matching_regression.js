/* Audit excerpts from 88lin/video_vip/video_vip.user.js (GPL-3.0).
 * Git blob SHA: 30260d8fb3db47b4f14fcc1a3faf8efadf99a43a
 * Functions copied from source lines 349-365, 521-529, 538-552.
 * Executed only against fixtures: no DOM mutation, no remote script execution.
 */
'use strict';
const fs = require('fs');
const _SITE_NAMES = '爱奇艺|腾讯视频|腾讯|优酷|芒果TV|芒果|哔哩哔哩|bilibili|B站|搜狐视频|搜狐|乐视视频|乐视|PPTV聚力|PPTV|1905电影网|1905|土豆网|土豆|风行网|风行|西瓜视频|咪咕视频|咪咕|AcFun';
const _SITE_NAME_RE = new RegExp('(?:^|[\\s\\-_：:|｜，,。·]+)(?:' + _SITE_NAMES + ')(?=$|[\\s\\-_：:|｜，,。·]+)', 'gi');
const _SITE_LEAD_RE = new RegExp('^(?:' + _SITE_NAMES + ')+', 'i');
const _SITE_TAIL_RE = new RegExp('(?:[\\s\\-_：:|｜]+|^)(?:' + _SITE_NAMES + ')$', 'i');
const _SITE_WORD_RE = new RegExp('^(?:' + _SITE_NAMES + ')$', 'i');
function wsyzyCleanTitle(t) {
    t = (t || '').replace(/[《》【】「」]/g, '')
        .replace(_SITE_NAME_RE, ' ')
        .replace(_SITE_LEAD_RE, '')
        .replace(_SITE_TAIL_RE, '')
        .replace(/在线观看|高清正版|免费观看|完整版|正片|预告|全集/g, '');
    const parts = t.split(/[-_\s（(|｜]/)
        .map(s => s.replace(/第.+[集季部]/, '').trim())
        .filter(Boolean);
    return parts.find(p => p.length >= 2 && !_SITE_WORD_RE.test(p) && !/第\d{1,8}[集期话季]/.test(p)) || parts[0] || '';
}
function parseEps(pu) {
    if (!pu) return [];
    return pu.split('$$$')[0].split('#')
        .map(s => {
            const i = s.indexOf('$');
            return i > 0 ? { name: s.slice(0, i), url: s.slice(i + 1) } : { name: '', url: s };
        })
        .filter(e => /^https?:\/\//.test(e.url));
}
let location = {href:''}, document = {title:''};
function curEpNum() {
    let m;
    m = location.href.match(/[?&](?:ep|episode|p|e|cur)=(\d{1,5})(?!\d)/i);
    if (m) return parseInt(m[1], 10);
    m = location.href.match(/\/p(\d{1,5})\.html/i);
    if (m) return parseInt(m[1], 10);
    m = location.href.match(/\/ep(\d{1,5})(?!\d)/i);
    if (m) return parseInt(m[1], 10);
    m = (document.title + ' ' + location.href).match(/第?\s*(\d{1,8})\s*[集期话]/);
    if (m) return parseInt(m[1], 10);
    return 0;
}
const tests=[];
function check(id,category,input,expected,actual,note=''){
 tests.push({id,category,input,expected,actual,pass:JSON.stringify(actual)===JSON.stringify(expected),note});
}
check('T01','control','示例剧 第12集 - 腾讯视频','示例剧',wsyzyCleanTitle('示例剧 第12集 - 腾讯视频'));
check('T02','title','Example Journey - 腾讯视频','Example Journey',wsyzyCleanTitle('Example Journey - 腾讯视频'),'Synthetic English title; meaningful words must not be dropped.');
check('T03','season','示例剧 第二季 - 腾讯视频','示例剧 第二季',wsyzyCleanTitle('示例剧 第二季 - 腾讯视频'),'Synthetic title; removing season loses matching information.');
check('T04','season','示例剧第二季 - 腾讯视频','示例剧第二季',wsyzyCleanTitle('示例剧第二季 - 腾讯视频'));
location.href='https://www.bilibili.com/bangumi/play/ep15014';
document.title='我们仍未知道那天所看见的花的名字。第1集-番剧-哔哩哔哩';
check('T05','episode',location.href,1,curEpNum(),'Official page identifies this as episode 1; ep15014 is an identifier, not ordinal 15014.');
location.href='https://www.bilibili.com/bangumi/play/ep114737'; document.title='头条都是他第1集';
check('T06','control',location.href,1,curEpNum(),'Six-digit identifier is skipped by the regex; title fallback works for this fixture.');
location.href='https://example.invalid/play/xyz';document.title='示例剧 第十二集';
check('T07','episode',document.title,12,curEpNum(),'Chinese numerals are not recognized. Synthetic input.');
const multi='第1集$https://first.example.invalid/e1.m3u8$$$第1集$https://second.example.invalid/e1.m3u8';
check('T08','sources','two $$$-separated groups',['https://first.example.invalid/e1.m3u8','https://second.example.invalid/e1.m3u8'],parseEps(multi).map(x=>x.url),'Second source group is discarded; not an uptime assertion.');
check('T09','control','one group / two episodes',2,parseEps('第1集$https://a.example.invalid/1.m3u8#第2集$https://a.example.invalid/2.m3u8').length);
check('T10','control','non-http media URL',0,parseEps('坏地址$javascript:alert(1)').length);
const report={source_blob:'30260d8fb3db47b4f14fcc1a3faf8efadf99a43a',execution:'Node.js / isolated source excerpts / fixtures, NOT live catalog or video playback',total:tests.length,passed:tests.filter(x=>x.pass).length,failed:tests.filter(x=>!x.pass).length,tests};
console.log(JSON.stringify(report,null,2));
fs.writeFileSync(__dirname+'/matching_results.json',JSON.stringify(report,null,2));
