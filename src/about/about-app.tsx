import type { ReactNode } from "react"
import { useEffect } from "react"
import { ArrowRight, ArrowUpRight, CodeXml, Scale } from "lucide-react"

import { version } from "../../package.json"
import { useAppSettings } from "@/app-settings"
import { ExternalLink } from "@/site/links"
import { REPOSITORY_URL } from "@/site/repository"
import { SiteFooter, SiteHeader, SkipLink } from "@/site/site-shell"
import "./about.css"

const AUTHOR = "yexca"
const LICENSE_URL = `${REPOSITORY_URL}/blob/main/LICENSE`
const CONTRIBUTORS_URL = `${REPOSITORY_URL}/graphs/contributors`
/** GitHub logins; avatars load from GitHub. */
const contributors = ["kanomahoro1224"]
const LINEBLOG_RECOVERY_URL =
  "https://github.com/ZhFuwe/Kano_LINE_Blog_Recovery"
const ZHFUWE = "ZhFuwe"
const ZHFUWE_URL = "https://github.com/ZhFuwe"
const MAINLAND_PROVIDER = "鹿可可"
const MAINLAND_PROVIDER_URL = "https://space.bilibili.com/426276698"
const MAINLAND_SITE_URL = "https://xiaolubao.com"
const developmentModels = ["Claude Opus 5.5", "GPT-6.1-Sol", "GPT-5.6-Sol"]
const technologyGroups = [
  {
    key: "frontend",
    items: [
      "React",
      "TypeScript",
      "Vite",
      "Tailwind CSS",
      "Radix UI",
      "lucide-react",
    ],
  },
  {
    key: "backend",
    items: ["Go", "net/http", "SQLite", "modernc.org/sqlite", "yt-dlp", "MCP"],
  },
  { key: "delivery", items: ["Docker", "Docker Compose", "GitHub Actions"] },
]

function Chapter({
  number,
  title,
  children,
}: {
  number: number
  title: ReactNode
  children?: ReactNode
}) {
  const id = `about-chapter-${number}`
  return (
    <section className="chapter" aria-labelledby={id}>
      <h2 id={id}>
        <span className="chapter-number" aria-hidden="true">
          {String(number).padStart(2, "0")}
        </span>
        {title}
      </h2>
      <div className="chapter-body">{children}</div>
    </section>
  )
}

/** A message with one of its values as a link to that person's page. */
function CreditText({
  message,
  param,
  name,
  href,
}: {
  message: string
  param: string
  name: string
  href: string
}) {
  const { t } = useAppSettings()
  const marker = "__CREDIT_NAME__"
  const [before, after = ""] = t(message, { [param]: marker }).split(marker)
  return (
    <p>
      {before}
      <ExternalLink className="about-inline-link" href={href}>
        {name}
      </ExternalLink>
      {after}
    </p>
  )
}

/** Thanks the person who helped build the Works page. */
function WorksCredit() {
  const { t } = useAppSettings()
  return (
    <>
      <CreditText
        message="about.creditsWorks"
        param="name"
        name={ZHFUWE}
        href={ZHFUWE_URL}
      />
      <a className="text-link" href="/works">
        {t("about.creditsWorksPage")}
        <ArrowRight aria-hidden="true" />
      </a>
    </>
  )
}

/** Credits the provider of the mainland China server site. */
function MainlandSiteCredit() {
  const { t } = useAppSettings()
  return (
    <>
      <CreditText
        message="about.creditsMainland"
        param="provider"
        name={MAINLAND_PROVIDER}
        href={MAINLAND_PROVIDER_URL}
      />
      <ExternalLink className="text-link" href={MAINLAND_SITE_URL}>
        {t("about.creditsMainlandSite")}
        <ArrowUpRight aria-hidden="true" />
      </ExternalLink>
    </>
  )
}

export function AboutApp() {
  const { t } = useAppSettings()

  useEffect(() => {
    document.title = t("about.metaTitle")
    document
      .querySelector('meta[name="description"]')
      ?.setAttribute("content", t("about.metaDescription"))
  }, [t])

  return (
    <div className="site about" id="top">
      <SkipLink href="#about-content">{t("about.skip")}</SkipLink>
      <SiteHeader page="about" />

      <main className="site-main" id="about-content">
        <section className="about-hero" aria-labelledby="about-title">
          <p className="kicker">{t("about.kicker")}</p>
          <h1 id="about-title">
            {t("site.name")}
            <span aria-hidden="true">🍓🦌</span>
          </h1>
          <p className="about-edition">
            <span className="tag">Kano Navi v{version}</span>
            <span className="tag">{t("about.tagline")}</span>
            <span className="tag">{t("about.by", { author: AUTHOR })}</span>
          </p>
          <nav className="about-links" aria-label={t("about.projectLinks")}>
            <ExternalLink
              className="pill-button is-solid"
              href={REPOSITORY_URL}
            >
              <CodeXml aria-hidden="true" />
              {t("about.sourceCode")}
              <ArrowUpRight aria-hidden="true" />
            </ExternalLink>
            <ExternalLink className="pill-button" href={LICENSE_URL}>
              <Scale aria-hidden="true" />
              {t("about.license")} · AGPL-3.0
              <ArrowUpRight aria-hidden="true" />
            </ExternalLink>
          </nav>
        </section>

        <div className="chapters">
          <Chapter number={1} title={t("about.overview")}>
            <p>{t("about.overviewText")}</p>
          </Chapter>
          <Chapter number={2} title={t("about.builtWithAi")}>
            <p>{t("about.aiCredit", { author: AUTHOR })}</p>
            <ul className="about-models" aria-label={t("about.models")}>
              {developmentModels.map((model) => (
                <li key={model}>{model}</li>
              ))}
            </ul>
          </Chapter>
          <Chapter number={3} title={t("about.contributors")}>
            <p>{t("about.contributorsText")}</p>
            <ul
              className="about-contributors"
              aria-label={t("about.contributorList")}
            >
              {contributors.map((login) => (
                <li key={login}>
                  <ExternalLink href={`https://github.com/${login}`}>
                    <img
                      src={`https://github.com/${login}.png?size=128`}
                      alt=""
                      referrerPolicy="no-referrer"
                      width={64}
                      height={64}
                      loading="lazy"
                      decoding="async"
                    />
                    <span>{login}</span>
                  </ExternalLink>
                </li>
              ))}
            </ul>
            <ExternalLink className="text-link" href={CONTRIBUTORS_URL}>
              {t("about.contributorsAll")}
              <ArrowUpRight aria-hidden="true" />
            </ExternalLink>
          </Chapter>
          <Chapter number={4} title={t("about.credits")}>
            <CreditText
              message="about.creditsLineblog"
              param="name"
              name={ZHFUWE}
              href={ZHFUWE_URL}
            />
            <ExternalLink className="text-link" href={LINEBLOG_RECOVERY_URL}>
              {t("about.creditsRecovery")}
              <ArrowUpRight aria-hidden="true" />
            </ExternalLink>
            <WorksCredit />
            <MainlandSiteCredit />
          </Chapter>
          <Chapter number={5} title={t("about.technologies")}>
            <dl className="about-stack">
              {technologyGroups.map(({ key, items }) => (
                <div key={key}>
                  <dt>{t(`about.group.${key}`)}</dt>
                  <dd>
                    <ul>
                      {items.map((item) => (
                        <li key={item}>{item}</li>
                      ))}
                    </ul>
                  </dd>
                </div>
              ))}
            </dl>
          </Chapter>
          <Chapter number={6} title={t("about.notice")}>
            <p>{t("about.unofficial")}</p>
            <p>{t("about.accuracy")}</p>
            <p>{t("about.rights")}</p>
            <p>
              {t("about.licenseText")}{" "}
              <ExternalLink className="text-link" href={LICENSE_URL}>
                {t("about.fullLicense")}
                <ArrowUpRight aria-hidden="true" />
              </ExternalLink>
            </p>
          </Chapter>
        </div>
      </main>

      <SiteFooter disclaimer={t("footer.disclaimer")} />
    </div>
  )
}
