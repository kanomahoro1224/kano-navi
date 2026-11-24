import type { Dashboard, Profile } from "@/types"

const X_ACCOUNT = /^https:\/\/(?:www\.)?(?:x|twitter)\.com\/\w+$/u

/**
 * The links the profile card carries: every X account in the link
 * directory, main first (the profile's own account when the directory has
 * none), the YouTube channel and the first bilibili space. The board's
 * links section leaves these out.
 */
export function profileLinks(
  profile: Pick<Profile, "xUrl" | "youtubeUrl">,
  resources: Dashboard["resources"],
) {
  const xUrls = resources
    .map((resource) => resource.url || "")
    .filter((url) => X_ACCOUNT.test(url))
  if (!xUrls.length) xUrls.push(profile.xUrl)
  const bilibili = resources.find((resource) =>
    /bilibili\.com/u.test(resource.url || ""),
  )?.url
  return { xUrls, youtube: profile.youtubeUrl, bilibili }
}

/** Every URL the profile card links to. */
export function profileLinkUrls(
  profile: Pick<Profile, "xUrl" | "youtubeUrl">,
  resources: Dashboard["resources"],
) {
  const { xUrls, youtube, bilibili } = profileLinks(profile, resources)
  return new Set([...xUrls, youtube, ...(bilibili ? [bilibili] : [])])
}
