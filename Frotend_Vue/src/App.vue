<!--
|--------------------------------------------------------------------------
| MeshHub 开源模型社区客户端主界面
|--------------------------------------------------------------------------
| 负责开源模型社区首页、个人项目列表、动态流、公开模型库和模型预览交互。
|--------------------------------------------------------------------------
-->
<script setup>
import {
  computed,
  nextTick,
  onMounted,
  onBeforeUnmount,
  ref,
  shallowRef
} from "vue";
import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { FBXLoader } from "three/examples/jsm/loaders/FBXLoader.js";
import { OBJLoader } from "three/examples/jsm/loaders/OBJLoader.js";
import { STLLoader } from "three/examples/jsm/loaders/STLLoader.js";

/*
|--------------------------------------------------------------------------
| 交互控件清单
|--------------------------------------------------------------------------
| 文件菜单按钮：展开导入文件、文件格式转化入口。
| 设置菜单按钮：展示当前客户端版本号。
| 品牌标识区域：展示 MeshHub 小鸡线稿 logo 和软件名称。
| 左侧个人项目面板：展示当前账户自己的开源模型项目。
| 中间动态面板：展示最近上传的模型作品和评论。
| 中间公开模型库面板：展示其他用户公开分享的模型作品。
| 右侧竖向小菜单：在“动态”和“开源模型库”之间切换中间内容。
| 导入文件按钮：通过后端分析或浏览器 fallback 选择三维模型文件。
| 上传云端按钮：选择三维模型并上传到当前账户的云端模型库。
| 转换格式弹窗：选择目标格式后，请求服务端转换当前云端模型。
| 进度弹窗：展示登录同步和上传云端的任务进度。
| 当前模型预览面板：展示选中模型，并支持旋转、拖拽查看和滚轮缩放。
| 放大按钮：拉近相机视角。
| 缩小按钮：拉远相机视角。
| 重置视图按钮：重新适配当前模型到预览区域。
| 云端登录能力：只在使用云端功能时需要账户，不再阻止进入客户端主页。
|--------------------------------------------------------------------------
*/

/*
|--------------------------------------------------------------------------
| 客户端版本信息
|--------------------------------------------------------------------------
| 设置菜单展示的软件版本号，后续可以由构建系统或后端接口注入。
|--------------------------------------------------------------------------
*/
const clientVersion = "0.1.0";

/*
|--------------------------------------------------------------------------
| 云端服务地址
|--------------------------------------------------------------------------
| 客户端启动时可后台检查服务状态，云端上传和同步功能继续使用该地址。
|--------------------------------------------------------------------------
*/
const cloudServerUrl = "http://45.197.145.14:18888";

/*
|--------------------------------------------------------------------------
| 支持导入的三维文件格式
|--------------------------------------------------------------------------
| stl、obj、fbx 支持直接预览；stp/step 和 SolidWorks 原生格式需要服务端转换。
|--------------------------------------------------------------------------
*/
const previewableFormats = new Set([
  "stl",
  "obj",
  "fbx"
]);

const supportedFormats = [
  ".stp",
  ".step",
  ".stl",
  ".sldprt",
  ".sldasm",
  ".slddrw",
  ".obj",
  ".fbx"
];

/*
|--------------------------------------------------------------------------
| 界面状态
|--------------------------------------------------------------------------
| 保存菜单展开状态、导入文件列表、当前选中文件和预览状态提示。
|--------------------------------------------------------------------------
*/
const activeMenu      = ref("");
const importedFiles   = ref([]);
const selectedFileId  = ref("");
const communityView   = ref("activity");
const repositoryQuery = ref("");
const globalSearch    = ref("");
const homePrompt      = ref("");
const previewStatus   = ref("请通过左上角“文件 > 导入文件”添加三维模型。");
const previewError    = ref("");
const isViewerReady   = ref(false);
const fileInputRef    = ref(null);
const viewerCanvasRef = ref(null);
const fileInputMode   = ref("import");

/*
|--------------------------------------------------------------------------
| 云端任务状态
|--------------------------------------------------------------------------
| 保存登录同步、上传云端和格式转换弹窗状态，用进度条向用户展示长任务。
|--------------------------------------------------------------------------
*/
const cloudSyncProgress  = ref(0);
const uploadProgress     = ref(0);
const operationTitle     = ref("");
const operationStatus    = ref("");
const operationError     = ref("");
const operationProgress  = ref(0);
const isOperationVisible = ref(false);
const isConvertVisible   = ref(false);
const targetFormat       = ref("stl");
const convertStatus      = ref("");
const convertError       = ref("");

/*
|--------------------------------------------------------------------------
| 可选云端登录状态
|--------------------------------------------------------------------------
| 保存网络检测、服务端可达性、账户密码输入和登录令牌；不再作为客户端门禁。
|--------------------------------------------------------------------------
*/
const isNetworkOnline   = ref(navigator.onLine);
const isServerReachable = ref(false);
const accessStatus      = ref("正在检测网络和云端服务。");
const accessError       = ref("");
const accountName       = ref("");
const accountPassword   = ref("");
const authToken         = ref(localStorage.getItem("assetTranscoderToken") ?? "");
const authUser          = ref(localStorage.getItem("assetTranscoderUser") ?? "");

/*
|--------------------------------------------------------------------------
| 客户端主页访问状态
|--------------------------------------------------------------------------
| 主界面默认开放；云端账户只在上传、同步、转换等云端动作中按需使用。
|--------------------------------------------------------------------------
*/
const isClientUnlocked = computed(() => {
  return true;
});

/*
|--------------------------------------------------------------------------
| 社区首页展示数据
|--------------------------------------------------------------------------
| 公开模型库和动态流先使用前端展示数据，后续可替换为服务端社区接口。
|--------------------------------------------------------------------------
*/
const publicModelLibrary = [
  {
    id: "public-camera-rig",
    name: "模块化相机云台",
    owner: "robot-lab",
    format: "STEP",
    stars: 128,
    comments: 18,
    updatedAt: "今天 14:20",
    summary: "适合机器人视觉、桌面机械臂和教学项目的开源三维结构。"
  },
  {
    id: "public-gearbox",
    name: "轻量化减速箱外壳",
    owner: "mesh-maker",
    format: "STL",
    stars: 94,
    comments: 12,
    updatedAt: "昨天 22:06",
    summary: "面向 FDM 打印优化，保留装配定位孔和加强筋结构。"
  },
  {
    id: "public-drone-frame",
    name: "四旋翼快拆机架",
    owner: "open-aero",
    format: "FBX",
    stars: 211,
    comments: 36,
    updatedAt: "5 月 10 日",
    summary: "公开模型库热门项目，可用于结构参考和二次建模。"
  }
];

const communityComments = [
  {
    id: "comment-review-fit",
    author: "Ming",
    target: "智能车轮组件",
    content: "装配间隙标注很清楚，建议把轴承座单独拆成一个版本。",
    time: "12 分钟前"
  },
  {
    id: "comment-print-test",
    author: "Alex",
    target: "模块化相机云台",
    content: "我用 PETG 打印过，右侧支架可以再加一条加强筋。",
    time: "38 分钟前"
  }
];

const dashboardTimelineEntries = [
  {
    id: "timeline-launch",
    title: "MeshHub 社区首页改版",
    time: "今天",
    summary: "正在把桌面客户端调整为更接近 GitHub Dashboard 的仓库和动态流结构。"
  },
  {
    id: "timeline-convert",
    title: "云端模型格式转化",
    time: "昨天",
    summary: "支持把云端模型提交到服务端转换任务队列，并把结果写回模型库。"
  },
  {
    id: "timeline-preview",
    title: "本地模型实时预览",
    time: "5 月 10 日",
    summary: "桌面端已支持 STL、OBJ、FBX 直接预览，并保留 STEP 转换流程。"
  }
];

const personalProjects = computed(() => {
  return importedFiles.value.map((file) => {
    return {
      id: file.id,
      name: file.name,
      format: file.formatName || file.extension.toUpperCase(),
      visibility: file.source === "cloud" ? "云端项目" : "本地草稿",
      updatedAt: file.details?.find((detail) => detail.label === "创建时间")?.value || "刚刚更新"
    };
  });
});

const dashboardRepositories = computed(() => {
  if (personalProjects.value.length > 0) {
    return personalProjects.value.map((project) => {
      return {
        id: project.id,
        title: `${authUser.value || "MeshHub"}/${project.name}`,
        meta: `${project.format} · ${project.visibility}`,
        selectable: true
      };
    });
  }

  return publicModelLibrary.map((model) => {
    return {
      id: model.id,
      title: `${model.owner}/${model.name}`,
      meta: `${model.format} · ${model.stars} stars`,
      selectable: false
    };
  });
});

const filteredRepositories = computed(() => {
  const keyword = repositoryQuery.value.trim().toLowerCase();

  if (!keyword) {
    return dashboardRepositories.value;
  }

  return dashboardRepositories.value.filter((repository) => {
    return repository.title.toLowerCase().includes(keyword)
      || repository.meta.toLowerCase().includes(keyword);
  });
});

const recentUploads = computed(() => {
  if (importedFiles.value.length === 0) {
    return publicModelLibrary.slice(0, 2).map((model) => {
      return {
        id: `seed-${model.id}`,
        name: model.name,
        owner: model.owner,
        format: model.format,
        time: model.updatedAt,
        summary: model.summary,
        source: "public"
      };
    });
  }

  return importedFiles.value.slice(0, 5).map((file) => {
    return {
      id: file.id,
      name: file.name,
      owner: authUser.value || "me",
      format: file.formatName || file.extension.toUpperCase(),
      time: file.details?.find((detail) => detail.label === "创建时间")?.value || "刚刚上传",
      summary: file.summary,
      source: file.source || "local"
    };
  });
});

const dashboardFeedEntries = computed(() => {
  if (communityView.value === "library") {
    return publicModelLibrary.map((model) => {
      return {
        id: model.id,
        title: `${model.owner}/${model.name}`,
        meta: `${model.format} · ${model.updatedAt}`,
        summary: model.summary,
        stats: `${model.stars} stars · ${model.comments} 评论`,
        action: "Star"
      };
    });
  }

  return recentUploads.value.map((upload) => {
    return {
      id: upload.id,
      title: `${upload.owner}/${upload.name}`,
      meta: `${upload.format} · ${upload.time}`,
      summary: upload.summary,
      stats: upload.source === "public" ? "公开模型" : "我的项目",
      action: upload.source === "public" ? "Explore" : "Open"
    };
  });
});

/*
|--------------------------------------------------------------------------
| Three.js 预览运行时
|--------------------------------------------------------------------------
| 保存场景、相机、渲染器、控制器和当前模型，负责右侧预览区的交互渲染。
|--------------------------------------------------------------------------
*/
const scene          = shallowRef(null);
const camera         = shallowRef(null);
const renderer       = shallowRef(null);
const controls       = shallowRef(null);
const activeModel    = shallowRef(null);
const animationFrame = shallowRef(0);

/*
|--------------------------------------------------------------------------
| 顶部菜单样式
|--------------------------------------------------------------------------
| 文件、设置两个顶部菜单共用的按钮和下拉面板样式。
|--------------------------------------------------------------------------
*/
const menuButtonClass = [
  "rounded-lg",
  "px-3",
  "py-1.5",
  "text-sm",
  "font-medium",
  "text-app-text-muted",
  "transition",
  "hover:bg-app-surface-hover",
  "hover:text-app-text"
];

const menuPanelClass = [
  "absolute",
  "z-30",
  "min-w-48",
  "rounded-xl",
  "border",
  "border-app-border",
  "bg-app-surface",
  "p-2",
  "shadow-2xl",
  "shadow-black/30"
];

const menuItemClass = [
  "w-full",
  "rounded-lg",
  "px-3",
  "py-2",
  "text-left",
  "text-sm",
  "text-app-text-muted",
  "transition",
  "hover:bg-app-surface-hover",
  "hover:text-app-text"
];

/*
|--------------------------------------------------------------------------
| GitHub 风格仪表盘样式
|--------------------------------------------------------------------------
| 控制顶部导航、仓库侧栏、主页卡片、feed 和右侧状态栏。
|--------------------------------------------------------------------------
*/
const dashboardHeaderIconButtonClass = [
  "flex",
  "h-9",
  "w-9",
  "items-center",
  "justify-center",
  "rounded-md",
  "border",
  "border-app-border-soft",
  "bg-app-sidebar",
  "text-sm",
  "font-medium",
  "text-app-text-muted",
  "transition",
  "hover:border-app-border",
  "hover:text-app-text"
];

const dashboardHeaderSearchClass = [
  "h-9",
  "w-full",
  "rounded-md",
  "border",
  "border-app-border-soft",
  "bg-app-bg",
  "px-3",
  "text-sm",
  "text-app-text",
  "outline-none",
  "placeholder:text-app-text-subtle",
  "focus:border-app-border"
];

const dashboardHeaderActionClass = [
  "rounded-md",
  "border",
  "border-app-border-soft",
  "bg-app-sidebar",
  "px-3",
  "py-2",
  "text-sm",
  "font-medium",
  "text-app-text-muted",
  "transition",
  "hover:border-app-border",
  "hover:text-app-text"
];

const dashboardSidebarClass = [
  "flex",
  "h-full",
  "min-h-0",
  "w-[320px]",
  "shrink-0",
  "flex-col",
  "border-r",
  "border-app-border",
  "bg-app-sidebar"
];

const dashboardSidebarSearchClass = [
  "h-9",
  "w-full",
  "rounded-md",
  "border",
  "border-app-border-soft",
  "bg-app-bg",
  "px-3",
  "text-sm",
  "text-app-text",
  "outline-none",
  "placeholder:text-app-text-subtle",
  "focus:border-app-border"
];

const fileItemBaseClass = [
  "w-full",
  "rounded-md",
  "border",
  "p-3",
  "text-left",
  "transition"
];

const fileItemActiveClass = [
  "border-app-border",
  "bg-app-sidebar-soft"
];

const fileItemIdleClass = [
  "border-app-border-soft",
  "bg-transparent",
  "hover:border-app-border",
  "hover:bg-app-surface-hover/60"
];

/*
|--------------------------------------------------------------------------
| GitHub 风格主页卡片样式
|--------------------------------------------------------------------------
| 用于 Home 标题区、提问输入区、Feed 卡片和右侧面板。
|--------------------------------------------------------------------------
*/
const communityMainClass = [
  "min-w-0",
  "flex-1",
  "overflow-y-auto",
  "bg-app-bg",
  "px-8",
  "py-8"
];

const communityPanelClass = [
  "rounded-xl",
  "border",
  "border-app-border-soft",
  "bg-app-surface",
  "p-4",
  "shadow-sm",
  "shadow-black/10"
];

const communityCardClass = [
  "rounded-md",
  "border",
  "border-app-border-soft",
  "bg-app-surface",
  "p-4",
  "transition",
  "hover:border-app-border",
  "hover:bg-app-surface-hover"
];

const communityTabButtonBaseClass = [
  "flex",
  "items-center",
  "justify-center",
  "rounded-full",
  "border",
  "px-3",
  "py-1.5",
  "text-sm",
  "font-medium",
  "transition",
];

const communityTabActiveClass = [
  "border-app-border",
  "bg-app-surface-raised",
  "text-app-text"
];

const communityTabIdleClass = [
  "border-app-border-soft",
  "bg-app-bg",
  "text-app-text-muted",
  "hover:border-app-border",
  "hover:text-app-text"
];

/*
|--------------------------------------------------------------------------
| 预览控制按钮样式
|--------------------------------------------------------------------------
| 右侧三维预览区的缩放和重置按钮统一使用这组样式。
|--------------------------------------------------------------------------
*/
const viewerButtonClass = [
  "rounded-xl",
  "border",
  "border-app-border",
  "bg-app-surface/95",
  "px-3",
  "py-1.5",
  "text-sm",
  "font-medium",
  "text-app-text-muted",
  "transition",
  "hover:bg-app-surface-hover",
  "hover:text-app-text"
];

/*
|--------------------------------------------------------------------------
| 可选登录弹窗样式
|--------------------------------------------------------------------------
| 保留云端账户登录弹窗样式，后续可作为云端功能的按需登录入口。
|--------------------------------------------------------------------------
*/
const authPanelClass = [
  "w-full",
  "max-w-md",
  "rounded-2xl",
  "border",
  "border-app-border",
  "bg-app-surface",
  "p-5",
  "shadow-2xl",
  "shadow-black/40"
];

const authInputClass = [
  "w-full",
  "rounded-xl",
  "border",
  "border-app-border",
  "bg-app-bg",
  "px-3",
  "py-2.5",
  "text-sm",
  "text-app-text",
  "outline-none",
  "transition",
  "focus:border-app-accent"
];

const authButtonClass = [
  "w-full",
  "rounded-xl",
  "border",
  "border-app-border",
  "bg-app-surface-hover",
  "px-3",
  "py-2.5",
  "text-sm",
  "font-medium",
  "text-app-text",
  "transition",
  "hover:border-app-accent"
];

/*
|--------------------------------------------------------------------------
| 进度弹窗样式
|--------------------------------------------------------------------------
| 上传云端和同步云端模型统一使用该弹窗展示任务名称、状态和进度条。
|--------------------------------------------------------------------------
*/
const progressPanelClass = [
  "w-full",
  "max-w-md",
  "rounded-2xl",
  "border",
  "border-app-border",
  "bg-app-surface",
  "p-5",
  "shadow-2xl",
  "shadow-black/40"
];

const secondaryButtonClass = [
  "rounded-xl",
  "border",
  "border-app-border",
  "bg-app-surface-hover",
  "px-3",
  "py-2",
  "text-sm",
  "font-medium",
  "text-app-text",
  "transition",
  "hover:border-app-accent"
];

const targetFormatOptions = [
  "stl",
  "obj",
  "fbx"
];

/*
|--------------------------------------------------------------------------
| 当前选中文件
|--------------------------------------------------------------------------
| 根据左侧列表选中项，计算右侧预览面板正在显示的文件。
|--------------------------------------------------------------------------
*/
const selectedFile = computed(() => {
  return importedFiles.value.find((file) => file.id === selectedFileId.value) ?? null;
});

/*
|--------------------------------------------------------------------------
| 可选云端服务检测
|--------------------------------------------------------------------------
| 检测浏览器网络和云端服务；检测失败只影响云端能力，不阻止使用主页。
|--------------------------------------------------------------------------
*/
async function initializeAccessGate() {
  isNetworkOnline.value = true;
  await checkCloudServer();
}

async function checkCloudServer() {
  try {
    const response = await fetch(`${cloudServerUrl}/api/health`, {
      method: "GET",
      cache: "no-store"
    });

    isServerReachable.value = response.ok;
    accessStatus.value = response.ok
      ? (authToken.value ? "云端服务已连接，正在同步云端模型。" : "云端服务已连接，请输入账户密码。")
      : "云端服务不可用，请确认服务端已启动。";

    if (response.ok && authToken.value) {
      await syncCloudModelsAfterLogin();
    }
  } catch (error) {
    isServerReachable.value = false;
    accessStatus.value      = "无法连接云端服务，请先启动 server。";
    accessError.value       = formatErrorMessage(error);
  }
}

async function loginAccount() {
  accessError.value = "";

  if (!isServerReachable.value) {
    await checkCloudServer();
  }

  try {
    const response = await fetch(`${cloudServerUrl}/api/auth/login`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify({
        username: accountName.value,
        password: accountPassword.value
      })
    });

    const payload = await response.json();
    if (!response.ok) {
      throw new Error(payload.error || "登录失败。");
    }

    authToken.value = payload.token;
    authUser.value  = payload.username;
    localStorage.setItem("assetTranscoderToken", payload.token);
    localStorage.setItem("assetTranscoderUser", payload.username);
    accessStatus.value = `已登录：${payload.username}`;

    await syncCloudModelsAfterLogin();
  } catch (error) {
    accessError.value = formatErrorMessage(error);
  }
}

function handleNetworkOnline() {
  isNetworkOnline.value = true;
  initializeAccessGate();
}

function handleNetworkOffline() {
  isNetworkOnline.value = false;
  accessStatus.value    = "系统网络状态显示离线，正在保留云端服务检测结果。";
}

function clearLocalSession() {
  authToken.value = "";
  authUser.value  = "";
  localStorage.removeItem("assetTranscoderToken");
  localStorage.removeItem("assetTranscoderUser");
}

/*
|--------------------------------------------------------------------------
| 登录同步云端模型
|--------------------------------------------------------------------------
| 登录成功后拉取当前账户云端模型列表，并用进度弹窗展示同步过程。
|--------------------------------------------------------------------------
*/
async function syncCloudModelsAfterLogin() {
  showOperationProgress(
    "同步云端模型",
    "正在读取当前账户的云端模型库。",
    12
  );

  try {
    cloudSyncProgress.value = 35;
    updateOperationProgress("正在请求模型数据池。", 35);

    const models = await requestCloudModels();

    updateOperationProgress("正在整理云端模型列表。", 72);
    importedFiles.value = models.map(createCloudModelFileRecord);
    selectedFileId.value = importedFiles.value[0]?.id ?? "";

    updateOperationProgress("同步完成。", 100);
    accessStatus.value = `已登录：${authUser.value}，云端模型已同步。`;

    if (selectedFileId.value) {
      await selectModelFile(selectedFileId.value);
    }

    window.setTimeout(hideOperationProgress, 450);
  } catch (error) {
    operationError.value = formatErrorMessage(error);
    accessError.value    = `云端模型同步失败：${formatErrorMessage(error)}`;

    if (formatErrorMessage(error).includes("请先登录")) {
      clearLocalSession();
    }
  }
}

async function refreshCloudModelsSilently() {
  const models = await requestCloudModels();

  importedFiles.value = models.map(createCloudModelFileRecord);
  selectedFileId.value = importedFiles.value[0]?.id ?? selectedFileId.value;
}

/*
|--------------------------------------------------------------------------
| 文件菜单按钮
|--------------------------------------------------------------------------
| 控制顶部“文件”菜单展开，菜单内包含导入文件和格式转化入口。
|--------------------------------------------------------------------------
*/
function toggleFileMenu() {
  activeMenu.value = activeMenu.value === "file" ? "" : "file";
}

async function openImportDialog() {
  activeMenu.value = "";
  fileInputMode.value = "import";

  if (hasBackendModelApi()) {
    await importLocalModelsFromBackend();
    return;
  }

  fileInputRef.value?.click();
}

function openFormatConversion() {
  activeMenu.value   = "";
  convertError.value = "";
  convertStatus.value = selectedFile.value?.source === "cloud"
    ? "请选择目标格式，服务端会把转换结果写回当前账户模型库。"
    : "请先选择一个云端模型，再执行格式转换。";
  isConvertVisible.value = true;
}

/*
|--------------------------------------------------------------------------
| 上传云端按钮
|--------------------------------------------------------------------------
| 选择本地模型并上传到当前登录账户；上传完成后重新同步云端模型库。
|--------------------------------------------------------------------------
*/
async function openCloudUploadDialog() {
  activeMenu.value = "";
  fileInputMode.value = "upload";

  if (!authToken.value) {
    previewError.value = "请先登录账户后再上传云端。";
    return;
  }

  if (hasBackendModelApi()) {
    await uploadLocalModelsFromBackend();
    return;
  }

  fileInputRef.value?.click();
}

async function syncCloudModelsOnDemand() {
  if (!authToken.value) {
    await checkCloudServer();
    previewError.value = "当前未登录云端账户，暂时只能浏览本地和公开模型。";
    return;
  }

  await syncCloudModelsAfterLogin();
}

/*
|--------------------------------------------------------------------------
| 设置菜单按钮
|--------------------------------------------------------------------------
| 控制顶部“设置”菜单展开，用于展示当前客户端版本号。
|--------------------------------------------------------------------------
*/
function toggleSettingsMenu() {
  activeMenu.value = activeMenu.value === "settings" ? "" : "settings";
}

/*
|--------------------------------------------------------------------------
| 导入文件按钮
|--------------------------------------------------------------------------
| 优先调用 Wails 后端导入并分析本地模型；浏览器环境使用文件输入 fallback。
|--------------------------------------------------------------------------
*/
async function importLocalModelsFromBackend() {
  const appApi = getBackendAppApi();

  try {
    const analyses = await appApi.ImportModelFiles();

    if (!analyses || analyses.length === 0) {
      previewStatus.value = "未选择任何模型文件。";
      return;
    }

    const nextFiles = analyses.map(createBackendModelFileRecord);

    importedFiles.value = [
      ...importedFiles.value,
      ...nextFiles
    ];

    await selectModelFile(nextFiles[0].id);
  } catch (error) {
    previewError.value  = "后端本地模型分析失败。";
    previewStatus.value = formatErrorMessage(error);
  }
}

async function handleImportFiles(event) {
  const files = Array.from(event.target.files ?? []);

  event.target.value = "";

  if (files.length === 0) {
    return;
  }

  if (fileInputMode.value === "upload") {
    await uploadBrowserFilesToCloud(files);
    return;
  }

  const nextFiles = files
    .filter(isSupportedModelFile)
    .map(createModelFileRecord);

  if (nextFiles.length === 0) {
    previewError.value  = "未检测到支持的三维模型格式。";
    previewStatus.value = "请导入 stp、step、stl、sldprt、sldasm、slddrw、obj 或 fbx 文件。";
    return;
  }

  importedFiles.value = [
    ...importedFiles.value,
    ...nextFiles
  ];

  await selectModelFile(nextFiles[0].id);
}

async function uploadLocalModelsFromBackend() {
  const appApi = getBackendAppApi();

  try {
    const analyses = await appApi.ImportModelFiles();

    if (!analyses || analyses.length === 0) {
      previewStatus.value = "未选择任何要上传云端的模型文件。";
      return;
    }

    showOperationProgress("上传云端", "正在读取本地模型。", 4);

    for (let index = 0; index < analyses.length; index += 1) {
      const analysis = analyses[index];
      const payload  = await appApi.ReadModelForUpload(analysis.path);
      const blob     = new Blob(
        [decodeBase64ToBytes(payload.base64)],
        {
          type: payload.mimeType || "application/octet-stream"
        }
      );

      await uploadBlobToCloud(
        blob,
        payload.fileName,
        index,
        analyses.length
      );
    }

    updateOperationProgress("上传完成，正在同步云端模型。", 92);
    await syncCloudModelsAfterLogin();
  } catch (error) {
    operationError.value = formatErrorMessage(error);
    previewError.value   = `上传云端失败：${formatErrorMessage(error)}`;
  }
}

async function uploadBrowserFilesToCloud(files) {
  const supportedFiles = files.filter(isSupportedModelFile);

  if (supportedFiles.length === 0) {
    previewError.value = "未检测到支持上传的三维模型格式。";
    return;
  }

  showOperationProgress("上传云端", "正在上传模型到当前账户。", 4);

  try {
    for (let index = 0; index < supportedFiles.length; index += 1) {
      await uploadBlobToCloud(
        supportedFiles[index],
        supportedFiles[index].name,
        index,
        supportedFiles.length
      );
    }

    updateOperationProgress("上传完成，正在同步云端模型。", 92);
    await syncCloudModelsAfterLogin();
  } catch (error) {
    operationError.value = formatErrorMessage(error);
    previewError.value   = `上传云端失败：${formatErrorMessage(error)}`;
  }
}

/*
|--------------------------------------------------------------------------
| 文件列表面板
|--------------------------------------------------------------------------
| 点击左侧文件卡片后，切换当前选中文件并刷新右侧三维预览内容。
|--------------------------------------------------------------------------
*/
async function selectModelFile(fileId) {
  selectedFileId.value = fileId;
  previewError.value   = "";

  await nextTick();
  await renderSelectedModel();
}

function getFileItemClass(fileId) {
  return [
    ...fileItemBaseClass,
    ...(fileId === selectedFileId.value ? fileItemActiveClass : fileItemIdleClass)
  ];
}

/*
|--------------------------------------------------------------------------
| 顶部视图切换标签
|--------------------------------------------------------------------------
| 控制 Home 页面在“动态”和“开源模型库”之间切换。
|--------------------------------------------------------------------------
*/
function switchCommunityView(nextView) {
  communityView.value = nextView;
}

function getCommunityTabClass(viewName) {
  return [
    ...communityTabButtonBaseClass,
    ...(communityView.value === viewName ? communityTabActiveClass : communityTabIdleClass)
  ];
}

async function handleRepositorySelect(repository) {
  if (repository.selectable) {
    await selectModelFile(repository.id);
    return;
  }

  switchCommunityView("library");
}

/*
|--------------------------------------------------------------------------
| 放大按钮
|--------------------------------------------------------------------------
| 拉近相机位置，实现只影响预览视角的模型放大效果。
|--------------------------------------------------------------------------
*/
function zoomInPreview() {
  zoomPreview(0.82);
}

/*
|--------------------------------------------------------------------------
| 缩小按钮
|--------------------------------------------------------------------------
| 拉远相机位置，实现只影响预览视角的模型缩小效果。
|--------------------------------------------------------------------------
*/
function zoomOutPreview() {
  zoomPreview(1.22);
}

/*
|--------------------------------------------------------------------------
| 重置视图按钮
|--------------------------------------------------------------------------
| 将相机重新适配到当前模型范围，恢复适合观察的默认预览角度。
|--------------------------------------------------------------------------
*/
function resetPreviewView() {
  if (!activeModel.value) {
    return;
  }

  fitCameraToModel(activeModel.value);
}

/*
|--------------------------------------------------------------------------
| 转换格式弹窗
|--------------------------------------------------------------------------
| 仅转换云端模型，转换结果由服务端写回用户模型库，再同步回客户端列表。
|--------------------------------------------------------------------------
*/
function closeConvertDialog() {
  isConvertVisible.value = false;
}

async function submitCloudConversion() {
  convertError.value  = "";
  convertStatus.value = "";

  if (!selectedFile.value || selectedFile.value.source !== "cloud") {
    convertError.value = "请先在左侧选择一个云端模型。";
    return;
  }

  try {
    showOperationProgress(
      "格式转换",
      `正在请求服务端转换为 ${targetFormat.value.toUpperCase()}。`,
      18
    );

    const result = await requestCloudConversion(
      selectedFile.value.cloudId,
      targetFormat.value
    );

    const job = result.job;
    if (!job?.id) {
      throw new Error("服务端未返回转换任务编号。");
    }

    updateOperationProgress(result.message || "转换任务已进入队列。", job.progress || 20);

    await pollCloudConversionJob(job.id);
    await refreshCloudModelsSilently();
    updateOperationProgress("转换完成，模型列表已更新。", 100);
    convertStatus.value    = "转换完成，结果已加入云端模型库。";
    isConvertVisible.value = false;
    window.setTimeout(hideOperationProgress, 450);
  } catch (error) {
    operationError.value = formatErrorMessage(error);
    convertError.value   = formatErrorMessage(error);
  }
}

async function pollCloudConversionJob(jobId) {
  for (let attempt = 0; attempt < 240; attempt += 1) {
    const result = await requestCloudConversionStatus(jobId);
    const job    = result.job;

    if (!job) {
      throw new Error("服务端未返回转换任务状态。");
    }

    updateOperationProgress(
      job.error || job.message || "正在转换模型格式。",
      Math.max(20, Math.min(100, job.progress || 20))
    );

    if (job.status === "done") {
      return job;
    }

    if (job.status === "failed") {
      throw new Error(job.error || "服务端转换失败。");
    }

    await waitForMilliseconds(1200);
  }

  throw new Error("转换任务等待超时。");
}

/*
|--------------------------------------------------------------------------
| 三维预览初始化
|--------------------------------------------------------------------------
| 创建 Three.js 场景、相机、灯光、控制器和渲染循环。
|--------------------------------------------------------------------------
*/
function initializeViewer() {
  if (renderer.value || !viewerCanvasRef.value) {
    return;
  }

  const canvas       = viewerCanvasRef.value;
  const canvasParent = canvas.parentElement;
  const width        = canvasParent.clientWidth;
  const height       = canvasParent.clientHeight;

  scene.value = new THREE.Scene();
  scene.value.background = new THREE.Color(0x151515);

  camera.value = new THREE.PerspectiveCamera(
    45,
    width / height,
    0.1,
    10000
  );
  camera.value.position.set(4, 3, 5);

  renderer.value = new THREE.WebGLRenderer({
    antialias: true,
    canvas
  });
  renderer.value.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  renderer.value.setSize(width, height);

  controls.value = new OrbitControls(camera.value, canvas);
  controls.value.enableDamping = true;
  controls.value.dampingFactor = 0.08;
  controls.value.enablePan     = true;
  controls.value.enableZoom    = true;

  addViewerLights();
  addViewerGrid();
  startViewerLoop();
  window.addEventListener("resize", resizeViewer);

  isViewerReady.value = true;
}

/*
|--------------------------------------------------------------------------
| 三维模型渲染
|--------------------------------------------------------------------------
| 根据当前文件格式选择 Three.js 加载器，完成模型预览或给出转换提示。
|--------------------------------------------------------------------------
*/
async function renderSelectedModel() {
  initializeViewer();
  clearActiveModel();

  if (!selectedFile.value) {
    previewStatus.value = "请选择左侧文件列表中的模型。";
    return;
  }

  if (!previewableFormats.has(selectedFile.value.extension)) {
    renderUnsupportedModelPlaceholder();
    return;
  }

  try {
    previewStatus.value = `正在加载 ${selectedFile.value.name} ...`;

    const model = await loadPreviewableModel(selectedFile.value);

    activeModel.value = model;
    scene.value.add(model);
    fitCameraToModel(model);

    previewStatus.value = "模型已加载，可拖拽旋转视角，也可使用滚轮或右上角按钮缩放。";
  } catch (error) {
    previewError.value  = "模型解析失败，请检查文件是否完整或格式是否受支持。";
    previewStatus.value = error instanceof Error ? error.message : "未知加载错误。";
  }
}

/*
|--------------------------------------------------------------------------
| 支持格式加载器
|--------------------------------------------------------------------------
| stl、obj、fbx 使用 Three.js 官方示例加载器完成前端预览。
|--------------------------------------------------------------------------
*/
async function loadPreviewableModel(fileRecord) {
  const objectUrl = fileRecord.file
    ? URL.createObjectURL(fileRecord.file)
    : await createPreviewObjectUrl(fileRecord);

  try {
    if (fileRecord.extension === "stl") {
      return await loadStlModel(objectUrl);
    }

    if (fileRecord.extension === "obj") {
      return await loadObjModel(objectUrl);
    }

    return await loadFbxModel(objectUrl);
  } finally {
    URL.revokeObjectURL(objectUrl);
  }
}

async function loadStlModel(objectUrl) {
  const geometry = await new STLLoader().loadAsync(objectUrl);
  const material = createDefaultMaterial();
  const mesh     = new THREE.Mesh(geometry, material);

  geometry.computeVertexNormals();

  return mesh;
}

async function loadObjModel(objectUrl) {
  const model = await new OBJLoader().loadAsync(objectUrl);

  applyDefaultMaterial(model);

  return model;
}

async function loadFbxModel(objectUrl) {
  const model = await new FBXLoader().loadAsync(objectUrl);

  applyDefaultMaterial(model);

  return model;
}

/*
|--------------------------------------------------------------------------
| 非前端直读格式提示
|--------------------------------------------------------------------------
| stp/step 和 SolidWorks 原生格式先显示占位说明，后续由服务端转换后再预览。
|--------------------------------------------------------------------------
*/
function renderUnsupportedModelPlaceholder() {
  const extension = selectedFile.value.extension.toUpperCase();

  previewStatus.value = selectedFile.value.summary
    || `${extension} 已在云端模型库中，需要先转换为 STL、OBJ 或 FBX 后才能预览。`;
  previewError.value = "";
}

/*
|--------------------------------------------------------------------------
| 共用 Three.js 工具函数
|--------------------------------------------------------------------------
| 放置多个预览控件都会使用的渲染、清理、缩放、适配和文件识别能力。
|--------------------------------------------------------------------------
*/
function addViewerLights() {
  const ambientLight = new THREE.AmbientLight(0xffffff, 1.25);
  const keyLight     = new THREE.DirectionalLight(0xffffff, 2.2);
  const fillLight    = new THREE.DirectionalLight(0xf08a3e, 0.65);

  keyLight.position.set(4, 8, 6);
  fillLight.position.set(-5, 3, -4);

  scene.value.add(ambientLight);
  scene.value.add(keyLight);
  scene.value.add(fillLight);
}

function addViewerGrid() {
  const grid = new THREE.GridHelper(
    12,
    24,
    0x3a3a35,
    0x252521
  );

  grid.position.y = -0.01;
  scene.value.add(grid);
}

function startViewerLoop() {
  const renderFrame = () => {
    controls.value?.update();
    renderer.value?.render(scene.value, camera.value);

    animationFrame.value = requestAnimationFrame(renderFrame);
  };

  renderFrame();
}

function resizeViewer() {
  if (!renderer.value || !camera.value || !viewerCanvasRef.value) {
    return;
  }

  const canvasParent = viewerCanvasRef.value.parentElement;
  const width        = canvasParent.clientWidth;
  const height       = canvasParent.clientHeight;

  camera.value.aspect = width / height;
  camera.value.updateProjectionMatrix();
  renderer.value.setSize(width, height);
}

function clearActiveModel() {
  if (!activeModel.value) {
    return;
  }

  scene.value?.remove(activeModel.value);
  activeModel.value.traverse?.((child) => {
    if (child.geometry) {
      child.geometry.dispose();
    }

    if (child.material) {
      disposeMaterial(child.material);
    }
  });
  activeModel.value = null;
}

function fitCameraToModel(model) {
  const box      = new THREE.Box3().setFromObject(model);
  const center   = box.getCenter(new THREE.Vector3());
  const size     = box.getSize(new THREE.Vector3());
  const maxSize  = Math.max(size.x, size.y, size.z) || 1;
  const distance = maxSize * 2.2;

  controls.value.target.copy(center);
  camera.value.position.set(
    center.x + distance,
    center.y + distance * 0.75,
    center.z + distance
  );
  camera.value.near = Math.max(distance / 100, 0.01);
  camera.value.far  = distance * 100;
  camera.value.updateProjectionMatrix();
  controls.value.update();
}

function zoomPreview(scale) {
  if (!camera.value || !controls.value) {
    return;
  }

  const direction = camera.value.position
    .clone()
    .sub(controls.value.target)
    .multiplyScalar(scale);

  camera.value.position.copy(
    controls.value.target.clone().add(direction)
  );
  controls.value.update();
}

function createDefaultMaterial() {
  return new THREE.MeshStandardMaterial({
    color: 0xb8b8ae,
    metalness: 0.18,
    roughness: 0.48
  });
}

function applyDefaultMaterial(model) {
  model.traverse((child) => {
    if (!child.isMesh) {
      return;
    }

    child.material = createDefaultMaterial();
  });
}

function disposeMaterial(material) {
  if (Array.isArray(material)) {
    material.forEach(disposeMaterial);
    return;
  }

  material.dispose?.();
}

function isSupportedModelFile(file) {
  return supportedFormats.includes(`.${getFileExtension(file.name)}`);
}

function createModelFileRecord(file) {
  const extension = getFileExtension(file.name);

  return {
    id: `${file.name}-${file.size}-${file.lastModified}-${crypto.randomUUID()}`,
    file,
    name: file.name,
    size: file.size,
    sizeLabel: formatFileSize(file.size),
    extension,
    formatName: extension.toUpperCase(),
    previewable: previewableFormats.has(extension),
    needsConversion: !previewableFormats.has(extension),
    summary: previewableFormats.has(extension)
      ? "浏览器 fallback 导入，可直接进行前端预览。"
      : "浏览器 fallback 导入，需要桌面后端转换后才能预览。",
    details: [
      {
        label: "导入来源",
        value: "浏览器文件输入"
      }
    ]
  };
}

function createBackendModelFileRecord(analysis) {
  return {
    id: analysis.id,
    path: analysis.path,
    name: analysis.name,
    size: analysis.size,
    sizeLabel: analysis.sizeLabel,
    extension: analysis.extension,
    formatName: analysis.formatName,
    formatFamily: analysis.formatFamily,
    previewable: analysis.previewable,
    needsConversion: analysis.needsConversion,
    previewFormat: analysis.previewFormat,
    summary: analysis.summary,
    details: analysis.details ?? [],
    bounds: analysis.bounds ?? null
  };
}

function createCloudModelFileRecord(model) {
  const extension = model.extension || getFileExtension(model.fileName);

  return {
    id: `cloud-${model.id}`,
    cloudId: model.id,
    source: "cloud",
    name: model.fileName,
    size: model.size,
    sizeLabel: model.sizeLabel || formatFileSize(model.size),
    extension,
    formatName: extension.toUpperCase(),
    previewable: Boolean(model.previewable),
    needsConversion: Boolean(model.needsConvert),
    convertedFrom: model.convertedFrom || 0,
    summary: model.previewable
      ? "云端模型，可按需下载到内存中预览，本地不保存模型副本。"
      : "云端模型，需要先通过服务端转换为 STL、OBJ 或 FBX 后预览。",
    details: [
      {
        label: "来源",
        value: "云端模型库"
      },
      {
        label: "模型 ID",
        value: String(model.id)
      },
      {
        label: "创建时间",
        value: model.createdAt || "-"
      }
    ]
  };
}

function getFileExtension(fileName) {
  return fileName.split(".").pop()?.toLowerCase() ?? "";
}

function formatFileSize(size) {
  if (size < 1024) {
    return `${size} B`;
  }

  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }

  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

function hasBackendModelApi() {
  const appApi = getBackendAppApi();

  return Boolean(appApi?.ImportModelFiles && appApi?.ReadPreviewModel);
}

function getBackendAppApi() {
  return window.go?.main?.App ?? null;
}

async function createPreviewObjectUrl(fileRecord) {
  if (fileRecord.source === "cloud") {
    const blob = await requestCloudModelBlob(fileRecord.cloudId);

    return URL.createObjectURL(blob);
  }

  const payload = await requestPreviewModelFromBackend(fileRecord.path);
  const bytes   = decodeBase64ToBytes(payload.base64);
  const blob    = new Blob([bytes], {
    type: payload.mimeType || "application/octet-stream"
  });

  return URL.createObjectURL(blob);
}

async function requestPreviewModelFromBackend(path) {
  const appApi = getBackendAppApi();

  if (!appApi?.ReadPreviewModel) {
    throw new Error("当前环境无法读取本地模型预览文件。");
  }

  return appApi.ReadPreviewModel(path);
}

function decodeBase64ToBytes(base64Text) {
  const binaryText = window.atob(base64Text);
  const bytes      = new Uint8Array(binaryText.length);

  for (let index = 0; index < binaryText.length; index += 1) {
    bytes[index] = binaryText.charCodeAt(index);
  }

  return bytes;
}

/*
|--------------------------------------------------------------------------
| 共用云端 API
|--------------------------------------------------------------------------
| 放置登录后同步、上传、下载和转换模型都会使用的云端请求能力。
|--------------------------------------------------------------------------
*/
async function requestCloudModels() {
  const response = await fetch(`${cloudServerUrl}/api/client/models`, {
    method: "GET",
    headers: buildAuthHeaders(),
    cache: "no-store"
  });

  return await parseCloudResponse(response);
}

async function requestCloudModelBlob(modelId) {
  const response = await fetch(`${cloudServerUrl}/api/client/models/download?id=${modelId}`, {
    method: "GET",
    headers: buildAuthHeaders(),
    cache: "no-store"
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new Error(payload.error || "下载云端模型失败。");
  }

  return await response.blob();
}

async function requestCloudConversion(modelId, nextFormat) {
  const response = await fetch(`${cloudServerUrl}/api/client/convert`, {
    method: "POST",
    headers: {
      ...buildAuthHeaders(),
      "Content-Type": "application/json"
    },
    body: JSON.stringify({
      modelId: modelId,
      targetFormat: nextFormat
    })
  });

  return await parseCloudResponse(response);
}

async function requestCloudConversionStatus(jobId) {
  const response = await fetch(`${cloudServerUrl}/api/client/convert/status?id=${jobId}`, {
    method: "GET",
    headers: buildAuthHeaders(),
    cache: "no-store"
  });

  return await parseCloudResponse(response);
}

function uploadBlobToCloud(blob, fileName, fileIndex, fileCount) {
  return uploadBlobToCloudByChunks(blob, fileName, fileIndex, fileCount);
}

async function uploadBlobToCloudByChunks(blob, fileName, fileIndex, fileCount) {
  const chunkSize   = 512 * 1024;
  const totalChunks = Math.max(1, Math.ceil(blob.size / chunkSize));
  const uploadId    = crypto.randomUUID();
  let latestPayload = null;

  for (let chunkIndex = 0; chunkIndex < totalChunks; chunkIndex += 1) {
    const start    = chunkIndex * chunkSize;
    const end      = Math.min(blob.size, start + chunkSize);
    const formData = new FormData();

    formData.append("uploadId", uploadId);
    formData.append("fileName", fileName);
    formData.append("chunkIndex", String(chunkIndex));
    formData.append("totalChunks", String(totalChunks));
    formData.append("chunk", blob.slice(start, end), `${fileName}.part-${chunkIndex}`);

    const payload = await uploadChunkWithRetry(formData, fileName, chunkIndex, totalChunks);

    latestPayload = payload;
    updateChunkUploadProgress(fileName, fileIndex, fileCount, chunkIndex + 1, totalChunks);
  }

  return latestPayload?.model ?? latestPayload;
}

async function uploadChunkWithRetry(formData, fileName, chunkIndex, totalChunks) {
  const maxAttempts = 5;

  for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
    try {
      const response = await fetch(`${cloudServerUrl}/api/client/models/chunk`, {
        method: "POST",
        headers: buildAuthHeaders(),
        body: formData
      });

      return await parseCloudResponse(response);
    } catch (error) {
      if (attempt === maxAttempts) {
        throw new Error(`上传 ${fileName} 的第 ${chunkIndex + 1}/${totalChunks} 个分片失败：${formatErrorMessage(error)}`);
      }

      updateOperationProgress(
        `第 ${chunkIndex + 1}/${totalChunks} 个分片连接中断，正在第 ${attempt + 1} 次重试。`,
        uploadProgress.value
      );
      await waitForMilliseconds(600 * attempt);
    }
  }

  throw new Error("上传分片失败。");
}

function updateChunkUploadProgress(fileName, fileIndex, fileCount, completedChunks, totalChunks) {
  const fileBaseProgress = (fileIndex / fileCount) * 86;
  const fileProgress     = (completedChunks / totalChunks) * (86 / fileCount);
  const nextProgress     = Math.min(90, Math.round(4 + fileBaseProgress + fileProgress));

  uploadProgress.value = nextProgress;
  updateOperationProgress(
    `正在上传 ${fileName}（${fileIndex + 1}/${fileCount}，分片 ${completedChunks}/${totalChunks}）。`,
    nextProgress
  );
}

function buildAuthHeaders() {
  return {
    Authorization: `Bearer ${authToken.value}`
  };
}

async function parseCloudResponse(response) {
  const payload = await response.json().catch(() => ({}));

  if (!response.ok) {
    throw new Error(payload.error || "云端服务请求失败。");
  }

  return payload;
}

function parseXHRJSON(text) {
  try {
    return JSON.parse(text || "{}");
  } catch {
    return {};
  }
}

function waitForMilliseconds(milliseconds) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, milliseconds);
  });
}

/*
|--------------------------------------------------------------------------
| 共用进度弹窗
|--------------------------------------------------------------------------
| 登录同步、上传和转换共用这组状态写入函数，保持弹窗表现一致。
|--------------------------------------------------------------------------
*/
function showOperationProgress(title, status, progress) {
  operationTitle.value     = title;
  operationStatus.value    = status;
  operationError.value     = "";
  operationProgress.value  = progress;
  isOperationVisible.value = true;
}

function updateOperationProgress(status, progress) {
  operationStatus.value   = status;
  operationProgress.value = progress;
}

function hideOperationProgress() {
  isOperationVisible.value = false;
}

function formatErrorMessage(error) {
  if (error instanceof Error) {
    return error.message;
  }

  if (typeof error === "string") {
    return error;
  }

  return "未知错误。";
}

onMounted(() => {
  window.addEventListener("online", handleNetworkOnline);
  window.addEventListener("offline", handleNetworkOffline);
  initializeAccessGate();
});

onBeforeUnmount(() => {
  window.removeEventListener("online", handleNetworkOnline);
  window.removeEventListener("offline", handleNetworkOffline);
  window.removeEventListener("resize", resizeViewer);
  cancelAnimationFrame(animationFrame.value);
  clearActiveModel();
  controls.value?.dispose();
  renderer.value?.dispose();
});
</script>

<template>
  <main class="flex h-screen min-h-screen flex-col overflow-hidden bg-app-bg text-app-text">
    <!--
    |--------------------------------------------------------------------------
    | 已停用的登录遮罩
    |--------------------------------------------------------------------------
    | 主页不再强制登录；该结构保留为未来云端功能按需登录入口。
    |--------------------------------------------------------------------------
    -->
    <div
      v-if  = "!isClientUnlocked"
      class = "fixed inset-0 z-50 flex items-center justify-center bg-black/55 px-4 backdrop-blur-md"
    >
      <form
        :class          = "authPanelClass"
        @submit.prevent = "loginAccount"
      >
        <p class="text-sm font-semibold text-app-text">账户登录</p>
        <p class="mt-2 text-sm leading-6 text-app-text-muted">
          客户端需要联网并连接云端服务。请输入服务端数据库中的账户和密码。
        </p>

        <div class="mt-4 space-y-3">
          <label class="block text-xs text-app-text-subtle">账户</label>
          <input
            v-model      = "accountName"
            :class       = "authInputClass"
            autocomplete = "username"
            type         = "text"
          />

          <label class="block text-xs text-app-text-subtle">密码</label>
          <input
            v-model      = "accountPassword"
            :class       = "authInputClass"
            autocomplete = "current-password"
            type         = "password"
          />
        </div>

        <button
          :class   = "authButtonClass"
          class    = "mt-4"
          type     = "submit"
          :disabled = "!isNetworkOnline"
        >
          登录并进入客户端
        </button>

        <p class="mt-3 text-xs leading-5 text-app-text-subtle">
          {{ accessStatus }}
        </p>

        <p
          v-if  = "accessError"
          class = "mt-2 text-xs text-app-accent"
        >
          {{ accessError }}
        </p>
      </form>
    </div>

    <!--
    |--------------------------------------------------------------------------
    | 云端任务进度弹窗
    |--------------------------------------------------------------------------
    | 上传云端、登录同步和格式转换时展示当前任务进度。
    |--------------------------------------------------------------------------
    -->
    <div
      v-if  = "isOperationVisible"
      class = "fixed inset-0 z-40 flex items-center justify-center bg-black/45 px-4 backdrop-blur-sm"
    >
      <div :class="progressPanelClass">
        <p class="text-sm font-semibold text-app-text">
          {{ operationTitle }}
        </p>

        <p class="mt-2 text-sm leading-6 text-app-text-muted">
          {{ operationStatus }}
        </p>

        <div class="mt-4 h-2 overflow-hidden rounded-full bg-app-bg">
          <div
            class = "h-full rounded-full bg-app-accent transition-all"
            :style = "{ width: `${operationProgress}%` }"
          ></div>
        </div>

        <p class="mt-2 text-right text-xs text-app-text-subtle">
          {{ operationProgress }}%
        </p>

        <p
          v-if  = "operationError"
          class = "mt-3 text-xs leading-5 text-app-accent"
        >
          {{ operationError }}
        </p>
      </div>
    </div>

    <!--
    |--------------------------------------------------------------------------
    | 文件格式转换弹窗
    |--------------------------------------------------------------------------
    | 选择目标格式，并请求服务端把云端模型转换后写回模型库。
    |--------------------------------------------------------------------------
    -->
    <div
      v-if  = "isConvertVisible"
      class = "fixed inset-0 z-40 flex items-center justify-center bg-black/45 px-4 backdrop-blur-sm"
    >
      <div :class="progressPanelClass">
        <p class="text-sm font-semibold text-app-text">文件格式转化</p>
        <p class="mt-2 text-sm leading-6 text-app-text-muted">
          {{ selectedFile?.source === "cloud" ? selectedFile.name : "请先在左侧选择云端模型。" }}
        </p>

        <label class="mt-4 block text-xs text-app-text-subtle">目标格式</label>
        <select
          v-model = "targetFormat"
          :class  = "authInputClass"
          class   = "mt-2"
        >
          <option
            v-for = "format in targetFormatOptions"
            :key  = "format"
            :value = "format"
          >
            {{ format.toUpperCase() }}
          </option>
        </select>

        <div class="mt-4 flex justify-end gap-2">
          <button
            :class = "secondaryButtonClass"
            type   = "button"
            @click = "closeConvertDialog"
          >
            取消
          </button>

          <button
            :class   = "secondaryButtonClass"
            type     = "button"
            :disabled = "selectedFile?.source !== 'cloud'"
            @click   = "submitCloudConversion"
          >
            开始转换
          </button>
        </div>

        <p class="mt-3 text-xs leading-5 text-app-text-subtle">
          {{ convertStatus }}
        </p>

        <p
          v-if  = "convertError"
          class = "mt-2 text-xs text-app-accent"
        >
          {{ convertError }}
        </p>
      </div>
    </div>

    <!--
    |--------------------------------------------------------------------------
    | GitHub 风格顶部导航
    |--------------------------------------------------------------------------
    | 提供全局搜索、品牌入口、文件操作菜单和设置入口。
    |--------------------------------------------------------------------------
    -->
    <header class="flex h-14 shrink-0 items-center border-b border-app-border bg-app-sidebar px-4">
      <div class="flex items-center gap-3">
        <button
          :class = "dashboardHeaderIconButtonClass"
          type   = "button"
        >
          =
        </button>

        <img
          alt   = "MeshHub logo"
          class = "h-8 w-8 rounded-full border border-app-border bg-white object-contain p-1"
          src   = "/meshhub-logo.png"
        />

        <div>
          <p class="text-sm font-semibold text-app-text">Dashboard</p>
        </div>
      </div>

      <div class="mx-6 min-w-0 flex-1">
        <input
          v-model      = "globalSearch"
          :class       = "dashboardHeaderSearchClass"
          autocomplete = "off"
          placeholder  = "Type / to search repositories, models and comments"
          type         = "text"
        />
      </div>

      <nav class="flex items-center gap-2">
        <div class="relative">
          <button
            :class = "dashboardHeaderActionClass"
            type   = "button"
            @click = "toggleFileMenu"
          >
            + New
          </button>

          <div
            v-if  = "activeMenu === 'file'"
            :class = "menuPanelClass"
            class  = "right-0 top-11"
          >
            <button
              :class = "menuItemClass"
              type   = "button"
              @click = "openImportDialog"
            >
              导入文件
            </button>

            <button
              :class = "menuItemClass"
              type   = "button"
              @click = "openCloudUploadDialog"
            >
              上传云端
            </button>

            <button
              :class = "menuItemClass"
              type   = "button"
              @click = "openFormatConversion"
            >
              文件格式转化
            </button>
          </div>
        </div>

        <div class="relative">
          <button
            :class = "dashboardHeaderActionClass"
            type   = "button"
            @click = "toggleSettingsMenu"
          >
            Settings
          </button>

          <div
            v-if  = "activeMenu === 'settings'"
            :class = "menuPanelClass"
            class  = "right-0 top-11"
          >
            <div class="rounded-lg px-3 py-2 text-sm text-app-text-muted">
              版本号：{{ clientVersion }}
            </div>

            <div class="rounded-lg px-3 py-2 text-xs text-app-text-subtle">
              {{ accessStatus }}
            </div>
          </div>
        </div>

        <button
          :class = "dashboardHeaderActionClass"
          type   = "button"
        >
          {{ authUser || "Guest" }}
        </button>
      </nav>

      <input
        ref      = "fileInputRef"
        accept   = ".stp,.step,.stl,.sldprt,.sldasm,.slddrw,.obj,.fbx"
        class    = "hidden"
        multiple
        type     = "file"
        @change  = "handleImportFiles"
      />
    </header>

    <section class="flex min-h-0 flex-1">
      <!--
      |--------------------------------------------------------------------------
      | 左侧仓库侧栏
      |--------------------------------------------------------------------------
      | 参考 GitHub Dashboard 的 Top repositories 结构，展示个人模型仓库和快速筛选。
      |--------------------------------------------------------------------------
      -->
      <aside :class="dashboardSidebarClass">
        <div class="border-b border-app-border px-4 py-6">
          <div class="flex items-center justify-between gap-3">
            <p class="text-base font-semibold text-app-text">Top repositories</p>

            <button
              :class = "dashboardHeaderActionClass"
              type   = "button"
              @click = "openImportDialog"
            >
              New
            </button>
          </div>

          <input
            v-model      = "repositoryQuery"
            :class       = "dashboardSidebarSearchClass"
            autocomplete = "off"
            class        = "mt-4"
            placeholder  = "Find a repository..."
            type         = "text"
          />
        </div>

        <div class="min-h-0 flex-1 overflow-y-auto px-3 py-3">
          <div
            v-if  = "filteredRepositories.length === 0"
            class = "rounded-md border border-dashed border-app-border px-3 py-4 text-sm leading-6 text-app-text-subtle"
          >
            没有匹配的模型仓库。
          </div>

          <div
            v-else
            class = "space-y-2"
          >
            <button
              v-for = "repository in filteredRepositories"
              :key  = "repository.id"
              :class = "getFileItemClass(repository.id)"
              type   = "button"
              @click = "handleRepositorySelect(repository)"
            >
              <p class="break-words text-left text-sm font-medium leading-6 text-app-text">
                {{ repository.title }}
              </p>

              <p class="mt-1 text-left text-xs text-app-text-subtle">
                {{ repository.meta }}
              </p>
            </button>
          </div>
        </div>
      </aside>

      <!--
      |--------------------------------------------------------------------------
      | 中间 Home 内容区
      |--------------------------------------------------------------------------
      | 参考 GitHub Home 的提问输入区、动作栏和 Feed，同时接入 MeshHub 预览能力。
      |--------------------------------------------------------------------------
      -->
      <section :class="communityMainClass">
        <div class="mx-auto flex max-w-4xl flex-col gap-6">
          <div class="flex items-end justify-between gap-4">
            <div>
              <h1 class="text-4xl font-semibold tracking-tight text-app-text">Home</h1>
              <p class="mt-2 text-sm text-app-text-muted">
                用 GitHub Dashboard 的形式管理你的开源模型仓库、动态和预览。
              </p>
            </div>

            <div class="flex items-center gap-2">
              <button
                :class = "getCommunityTabClass('activity')"
                type   = "button"
                @click = "switchCommunityView('activity')"
              >
                动态
              </button>

              <button
                :class = "getCommunityTabClass('library')"
                type   = "button"
                @click = "switchCommunityView('library')"
              >
                开源模型库
              </button>
            </div>
          </div>

          <section :class="communityPanelClass">
            <textarea
              v-model      = "homePrompt"
              class        = "min-h-28 w-full resize-none bg-transparent text-lg text-app-text outline-none placeholder:text-app-text-subtle"
              placeholder  = "Ask anything about your models, repositories, preview tasks or format conversion"
            ></textarea>

            <div class="mt-4 flex items-center justify-between gap-3 border-t border-app-border-soft pt-4">
              <div class="flex flex-wrap items-center gap-2">
                <button
                  :class = "dashboardHeaderActionClass"
                  type   = "button"
                >
                  Ask
                </button>

                <button
                  :class = "dashboardHeaderActionClass"
                  type   = "button"
                >
                  All repositories
                </button>

                <button
                  :class = "dashboardHeaderActionClass"
                  type   = "button"
                >
                  +
                </button>
              </div>

              <p class="text-sm text-app-text-subtle">
                {{ authUser || "Guest" }} · {{ isServerReachable ? "Cloud online" : "Cloud offline" }}
              </p>
            </div>
          </section>

          <div class="flex flex-wrap gap-3">
            <button
              :class = "dashboardHeaderActionClass"
              type   = "button"
              @click = "openImportDialog"
            >
              Import model
            </button>

            <button
              :class = "dashboardHeaderActionClass"
              type   = "button"
              @click = "openCloudUploadDialog"
            >
              Upload cloud
            </button>

            <button
              :class = "dashboardHeaderActionClass"
              type   = "button"
              @click = "openFormatConversion"
            >
              Convert
            </button>

            <button
              :class = "dashboardHeaderActionClass"
              type   = "button"
              @click = "syncCloudModelsOnDemand"
            >
              Sync cloud
            </button>
          </div>

          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-xl font-semibold text-app-text">Feed</p>
              <p class="mt-1 text-sm text-app-text-muted">
                {{
                  communityView === "activity"
                    ? "最近上传的模型、评论和当前仓库预览。"
                    : "公开模型库中的热门项目和最新更新。"
                }}
              </p>
            </div>

            <button
              :class = "dashboardHeaderActionClass"
              type   = "button"
            >
              Filter
            </button>
          </div>

          <div
            v-show = "communityView === 'activity'"
            class  = "grid gap-4"
          >
            <section :class="communityPanelClass">
              <div class="flex items-center justify-between gap-4">
                <div class="min-w-0">
                  <p class="truncate text-lg font-semibold text-app-text">
                    {{ selectedFile?.name || "Current preview" }}
                  </p>

                  <p class="mt-2 text-sm text-app-text-muted">
                    {{ previewStatus }}
                  </p>
                </div>

                <div class="flex shrink-0 items-center gap-2">
                  <button
                    :class = "viewerButtonClass"
                    type   = "button"
                    @click = "zoomInPreview"
                  >
                    Zoom in
                  </button>

                  <button
                    :class = "viewerButtonClass"
                    type   = "button"
                    @click = "zoomOutPreview"
                  >
                    Zoom out
                  </button>

                  <button
                    :class = "viewerButtonClass"
                    type   = "button"
                    @click = "resetPreviewView"
                  >
                    Reset
                  </button>
                </div>
              </div>

              <div class="relative mt-4 h-[320px] overflow-hidden rounded-md border border-app-border-soft bg-app-bg">
                <canvas
                  ref   = "viewerCanvasRef"
                  class = "h-full w-full"
                ></canvas>

                <div
                  v-if  = "!isViewerReady && importedFiles.length === 0"
                  class = "pointer-events-none absolute inset-0 flex items-center justify-center text-sm text-app-text-subtle"
                >
                  导入模型后将在这里显示预览
                </div>
              </div>

              <div
                v-if  = "selectedFile?.details?.length"
                class = "mt-4 grid gap-3 sm:grid-cols-2"
              >
                <div
                  v-for = "detail in selectedFile.details.slice(0, 4)"
                  :key  = "`${detail.label}-${detail.value}`"
                  class = "rounded-md border border-app-border-soft bg-app-bg px-3 py-2"
                >
                  <p class="text-xs text-app-text-subtle">{{ detail.label }}</p>
                  <p class="mt-1 break-all text-sm text-app-text-muted">{{ detail.value }}</p>
                </div>
              </div>

              <p
                v-if  = "previewError"
                class = "mt-3 text-xs text-red-400"
              >
                {{ previewError }}
              </p>
            </section>

            <section :class="communityPanelClass">
              <div class="space-y-0 overflow-hidden rounded-md border border-app-border-soft">
                <article
                  v-for = "entry in dashboardFeedEntries"
                  :key  = "entry.id"
                  class = "border-b border-app-border-soft bg-app-surface px-4 py-4 last:border-b-0"
                >
                  <div class="flex items-start justify-between gap-4">
                    <div class="min-w-0">
                      <p class="break-words text-base font-semibold text-app-text">
                        {{ entry.title }}
                      </p>

                      <p class="mt-2 text-sm text-app-text-muted">
                        {{ entry.summary }}
                      </p>

                      <div class="mt-3 flex flex-wrap gap-3 text-xs text-app-text-subtle">
                        <span>{{ entry.meta }}</span>
                        <span>{{ entry.stats }}</span>
                      </div>
                    </div>

                    <button
                      :class = "dashboardHeaderActionClass"
                      type   = "button"
                    >
                      {{ entry.action }}
                    </button>
                  </div>
                </article>
              </div>
            </section>

            <section :class="communityPanelClass">
              <p class="text-base font-semibold text-app-text">Recent comments</p>

              <div class="mt-4 space-y-3">
                <article
                  v-for = "comment in communityComments"
                  :key  = "comment.id"
                  :class = "communityCardClass"
                >
                  <p class="text-sm font-medium text-app-text">
                    {{ comment.author }}
                    <span class="font-normal text-app-text-subtle">commented on</span>
                    {{ comment.target }}
                  </p>

                  <p class="mt-2 text-sm leading-6 text-app-text-muted">
                    {{ comment.content }}
                  </p>

                  <p class="mt-2 text-xs text-app-text-subtle">
                    {{ comment.time }}
                  </p>
                </article>
              </div>
            </section>
          </div>

          <section
            v-show  = "communityView === 'library'"
            :class  = "communityPanelClass"
          >
            <div class="space-y-0 overflow-hidden rounded-md border border-app-border-soft">
              <article
                v-for = "entry in dashboardFeedEntries"
                :key  = "entry.id"
                class = "border-b border-app-border-soft bg-app-surface px-4 py-5 last:border-b-0"
              >
                <div class="flex items-start justify-between gap-4">
                  <div class="min-w-0">
                    <p class="break-words text-base font-semibold text-app-text">
                      {{ entry.title }}
                    </p>

                    <p class="mt-2 text-sm leading-6 text-app-text-muted">
                      {{ entry.summary }}
                    </p>

                    <div class="mt-3 flex flex-wrap gap-3 text-xs text-app-text-subtle">
                      <span>{{ entry.meta }}</span>
                      <span>{{ entry.stats }}</span>
                    </div>
                  </div>

                  <button
                    :class = "dashboardHeaderActionClass"
                    type   = "button"
                  >
                    {{ entry.action }}
                  </button>
                </div>
              </article>
            </div>
          </section>
        </div>
      </section>

      <!--
      |--------------------------------------------------------------------------
      | 右侧状态栏
      |--------------------------------------------------------------------------
      | 参考 GitHub Dashboard 的 changelog 侧栏，展示最近更新和当前工作区状态。
      |--------------------------------------------------------------------------
      -->
      <aside class="hidden h-full w-[320px] shrink-0 overflow-y-auto border-l border-app-border px-6 py-8 xl:block">
        <div class="space-y-4">
          <section :class="communityPanelClass">
            <p class="text-base font-semibold text-app-text">Latest from MeshHub</p>

            <div class="mt-5 space-y-5">
              <article
                v-for = "(entry, index) in dashboardTimelineEntries"
                :key  = "entry.id"
                class = "relative pl-6"
              >
                <span class="absolute left-0 top-1 h-2 w-2 rounded-full bg-app-border"></span>
                <span
                  v-if  = "index < dashboardTimelineEntries.length - 1"
                  class = "absolute left-[3px] top-3 h-[calc(100%+8px)] w-px bg-app-border-soft"
                ></span>

                <p class="text-xs text-app-text-subtle">{{ entry.time }}</p>
                <p class="mt-2 text-sm font-medium text-app-text">{{ entry.title }}</p>
                <p class="mt-2 text-sm leading-6 text-app-text-muted">{{ entry.summary }}</p>
              </article>
            </div>
          </section>

          <section :class="communityPanelClass">
            <p class="text-base font-semibold text-app-text">Workspace</p>

            <div class="mt-4 space-y-3 text-sm">
              <div class="rounded-md border border-app-border-soft bg-app-bg px-3 py-3">
                <p class="text-xs text-app-text-subtle">当前视图</p>
                <p class="mt-1 text-app-text">
                  {{ communityView === "activity" ? "动态" : "开源模型库" }}
                </p>
              </div>

              <div class="rounded-md border border-app-border-soft bg-app-bg px-3 py-3">
                <p class="text-xs text-app-text-subtle">云端状态</p>
                <p class="mt-1 text-app-text">
                  {{ isServerReachable ? "Cloud online" : "Cloud offline" }}
                </p>
              </div>

              <div class="rounded-md border border-app-border-soft bg-app-bg px-3 py-3">
                <p class="text-xs text-app-text-subtle">当前账户</p>
                <p class="mt-1 text-app-text">
                  {{ authUser || "Guest" }}
                </p>
              </div>

              <div class="rounded-md border border-app-border-soft bg-app-bg px-3 py-3">
                <p class="text-xs text-app-text-subtle">已载入模型</p>
                <p class="mt-1 text-app-text">
                  {{ importedFiles.length }} 个
                </p>
              </div>
            </div>
          </section>
        </div>
      </aside>
    </section>
  </main>
</template>
