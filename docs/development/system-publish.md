修改文章的状态可以让文章处于发布状态。

但是要让博客从代码状态转换为 html 或者网页，需要 hugo generate 才行。

这一点在 github action 中完成的。

我们的任务是添加整体发布功能。

整体发布，是将本地 hugo 博客所对应的 git repository 内容提交 git commit，然后 push 到远程仓库。

然后 github action 可以发布整个站点。
