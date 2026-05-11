/*
|--------------------------------------------------------------------------
| 服务端本地账户管理页面
|--------------------------------------------------------------------------
| 提供只给服务器本机访问的极简前端，用于创建和删除客户端账户。
|--------------------------------------------------------------------------
*/
package main

/*
|--------------------------------------------------------------------------
| 模块能力清单
|--------------------------------------------------------------------------
| 管理界面：展示账户创建表单和账户列表。
| 创建账户：提交账户密码到本机管理 API。
| 删除账户：删除账户及其对应的独立用户数据库。
|--------------------------------------------------------------------------
*/

const adminHTML = `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>MeshHub Server</title>
    <style>
      :root {
        color-scheme: dark;
        --bg: #151515;
        --panel: #202020;
        --panel-hover: #2a2a2a;
        --border: #3a3a35;
        --text: #f1f1ef;
        --muted: #aaa9a3;
        --subtle: #77766f;
        --accent: #f08a3e;
      }

      * {
        box-sizing: border-box;
      }

      body {
        margin: 0;
        min-height: 100vh;
        background: var(--bg);
        color: var(--text);
        font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }

      main {
        max-width: 880px;
        margin: 0 auto;
        padding: 48px 24px;
      }

      h1 {
        margin: 0;
        font-size: 24px;
      }

      p {
        color: var(--muted);
        line-height: 1.7;
      }

      section {
        margin-top: 20px;
        border: 1px solid var(--border);
        border-radius: 18px;
        background: var(--panel);
        padding: 18px;
      }

      label {
        display: block;
        margin-bottom: 8px;
        color: var(--muted);
        font-size: 13px;
      }

      input {
        width: 100%;
        margin-bottom: 14px;
        border: 1px solid var(--border);
        border-radius: 12px;
        background: #151515;
        color: var(--text);
        padding: 11px 12px;
        outline: none;
      }

      button {
        border: 1px solid var(--border);
        border-radius: 12px;
        background: var(--panel-hover);
        color: var(--text);
        cursor: pointer;
        padding: 10px 13px;
      }

      button:hover {
        border-color: var(--accent);
      }

      .account {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        border-top: 1px solid var(--border);
        padding: 12px 0;
      }

      .account:first-child {
        border-top: 0;
      }

      .meta {
        color: var(--subtle);
        font-size: 12px;
      }

      .danger {
        color: #ffb18b;
      }
    </style>
  </head>
  <body>
    <main>
      <h1>MeshHub 服务端账户管理</h1>
      <p>该页面只允许服务器本机访问。这里创建的账户用于客户端强制登录，每个账户会生成独立的模型数据库。</p>

      <section>
        <h2>添加账户</h2>
        <label>账户</label>
        <input id="username" autocomplete="username" />
        <label>密码</label>
        <input id="password" type="password" autocomplete="new-password" />
        <button onclick="createAccount()">添加账户</button>
      </section>

      <section>
        <h2>账户列表</h2>
        <div id="accounts"></div>
      </section>
    </main>

    <script>
      async function loadAccounts() {
        const response = await fetch("/api/admin/accounts");
        const accounts = await response.json();
        const root = document.querySelector("#accounts");

        root.innerHTML = accounts.map((account) => ` + "`" + `
          <div class="account">
            <div>
              <strong>${account.username}</strong>
              <div class="meta">${account.createdAt}</div>
            </div>
            <button class="danger" onclick="deleteAccount('${account.username}')">删除</button>
          </div>
        ` + "`" + `).join("") || "<p>暂无账户</p>";
      }

      async function createAccount() {
        const username = document.querySelector("#username").value;
        const password = document.querySelector("#password").value;

        const response = await fetch("/api/admin/accounts", {
          method: "POST",
          headers: {
            "Content-Type": "application/json"
          },
          body: JSON.stringify({ username, password })
        });

        if (!response.ok) {
          const error = await response.json();
          alert(error.error || "创建失败");
          return;
        }

        document.querySelector("#username").value = "";
        document.querySelector("#password").value = "";
        await loadAccounts();
      }

      async function deleteAccount(username) {
        if (!confirm("确认删除账户 " + username + "？该账户的模型数据库也会删除。")) {
          return;
        }

        await fetch("/api/admin/accounts?username=" + encodeURIComponent(username), {
          method: "DELETE"
        });
        await loadAccounts();
      }

      loadAccounts();
    </script>
  </body>
</html>`
