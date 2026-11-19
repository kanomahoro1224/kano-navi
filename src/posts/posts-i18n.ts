import { dashboardMessages } from "@/dashboard/dashboard-i18n"

// The page reuses the board's post wording (filters, cards, the viewer) and
// adds its own heading and link-preview copy.
const pageMessages = {
  "zh-CN": {
    "posts.metaTitle": "动态 · 鹿乃在做什么",
    "posts.metaDescription":
      "鹿乃 2010 年以来在 X 和哔哩哔哩上的动态，按月份和账号浏览。",
    "posts.skip": "跳到动态列表",
    "posts.title": "每一条动态",
    "posts.total": ({ count }: { count: any }) => `共 ${count} 条`,
  },
  ja: {
    "posts.metaTitle": "ポスト · 鹿乃ちゃん何してるか",
    "posts.metaDescription":
      "2010 年からの鹿乃の X と bilibili のポストを、月とアカウントごとに。",
    "posts.skip": "ポスト一覧へスキップ",
    "posts.title": "これまでのポスト",
    "posts.total": ({ count }: { count: any }) => `${count} 件`,
  },
  en: {
    "posts.metaTitle": "Posts · What’s Kano Up To?",
    "posts.metaDescription":
      "Kano's posts on X and bilibili since 2010, by month and by account.",
    "posts.skip": "Skip to the posts",
    "posts.title": "Every post",
    "posts.total": ({ count }: { count: any }) =>
      `${count} ${count === 1 ? "post" : "posts"}`,
  },
}

export const postsMessages = {
  "zh-CN": { ...dashboardMessages["zh-CN"], ...pageMessages["zh-CN"] },
  ja: { ...dashboardMessages.ja, ...pageMessages.ja },
  en: { ...dashboardMessages.en, ...pageMessages.en },
}
