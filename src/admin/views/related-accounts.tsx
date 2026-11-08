import { useEffect, useState, type SubmitEvent } from "react"
import { Newspaper, Plus, Save, Trash2, UsersRound } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp } from "@/admin/format"
import type { XRelatedAccount, XRelatedAccounts } from "@/admin/types"
import { Badge, Button, Field, IconButton, Loading, Panel } from "@/admin/ui"
import { useAdmin, useResource } from "@/admin/use-admin-data"

interface Draft {
  key: number
  handle: string
  keywords: string
}

const keywordsText = (keywords: string[]) => keywords.join("\n")

const parseKeywords = (text: string) =>
  text
    .split(/[\n,，]/u)
    .map((keyword) => keyword.trim())
    .filter(Boolean)

const draftsOf = (accounts: XRelatedAccount[]): Draft[] =>
  accounts.map((account, key) => ({
    key,
    handle: account.handle,
    keywords: keywordsText(account.keywords),
  }))

const accountsOf = (drafts: Draft[]): XRelatedAccount[] =>
  drafts.map((draft) => ({
    handle: draft.handle.trim().replace(/^@/u, ""),
    keywords: parseKeywords(draft.keywords),
  }))

/**
 * The related X accounts on their timer's Sources page: accounts such as the
 * agency whose own timelines each x_related run reads in full, through the
 * FxTwitter timeline only, into External posts. A status whose text contains
 * one of the account's keyword rules is promoted onto the board.
 */
export function RelatedAccountsPanel({
  navigate,
}: {
  navigate: ViewProps["navigate"]
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const related = useResource(() => api.relatedAccounts(), [])
  const [drafts, setDrafts] = useState<Draft[]>([])
  const [nextKey, setNextKey] = useState(0)
  const [busy, setBusy] = useState(false)
  const saved = related.data
  useEffect(() => {
    if (!saved) return
    setDrafts(draftsOf(saved.accounts))
    setNextKey(saved.accounts.length)
  }, [saved])
  if (!saved) return <Loading />
  const dirty =
    JSON.stringify(accountsOf(drafts)) !== JSON.stringify(saved.accounts)
  const set = (key: number, patch: Partial<Draft>) =>
    setDrafts((current) =>
      current.map((draft) =>
        draft.key === key ? { ...draft, ...patch } : draft,
      ),
    )
  const submit = async (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault()
    setBusy(true)
    const next: XRelatedAccounts | undefined = await act(
      () => api.updateRelatedAccounts(accountsOf(drafts)),
      "admin.related.saved",
    )
    setBusy(false)
    if (next) related.setData(next)
  }
  const statusOf = (handle: string) =>
    saved.status.find(
      (entry) =>
        entry.handle.toLowerCase() ===
        handle.trim().replace(/^@/u, "").toLowerCase(),
    )
  return (
    <Panel
      icon={UsersRound}
      title={t("admin.related.title")}
      description={t("admin.related.description")}
      actions={
        <Button
          size="sm"
          variant="ghost"
          icon={Newspaper}
          onClick={() => navigate("external")}
        >
          {t("admin.related.openExternal")}
        </Button>
      }
    >
      <form className="adm-form" onSubmit={submit}>
        {drafts.length ? (
          <ul className="adm-related-list">
            {drafts.map((draft) => {
              const status = statusOf(draft.handle)
              return (
                <li key={draft.key} className="adm-related-account">
                  <div className="adm-related-fields">
                    <Field label={t("admin.related.handle")}>
                      <span className="adm-input-prefix">
                        <span aria-hidden>@</span>
                        <input
                          value={draft.handle}
                          onChange={(e) =>
                            set(draft.key, { handle: e.target.value })
                          }
                          maxLength={31}
                          pattern="@?[A-Za-z0-9_]{1,30}"
                          required
                          spellCheck={false}
                          autoCapitalize="off"
                        />
                      </span>
                    </Field>
                    <Field
                      label={t("admin.related.keywords")}
                      hint={t("admin.related.keywordsHint")}
                    >
                      <textarea
                        value={draft.keywords}
                        onChange={(e) =>
                          set(draft.key, { keywords: e.target.value })
                        }
                        rows={3}
                      />
                    </Field>
                    <IconButton
                      icon={Trash2}
                      label={t("admin.related.remove")}
                      onClick={() =>
                        setDrafts((current) =>
                          current.filter((entry) => entry.key !== draft.key),
                        )
                      }
                    />
                  </div>
                  {status ? (
                    <span className="adm-chip-row">
                      <Badge>
                        {status.lastSuccessAt
                          ? t("admin.related.lastRead", {
                              time: japanStamp(status.lastSuccessAt),
                            })
                          : t("admin.related.neverRead")}
                      </Badge>
                      <Badge tone="sky">
                        {t("admin.related.counts", {
                          statuses: status.statuses,
                          promoted: status.promoted,
                        })}
                      </Badge>
                      {status.lastError ? (
                        <Badge tone="danger">{status.lastError}</Badge>
                      ) : null}
                    </span>
                  ) : null}
                </li>
              )
            })}
          </ul>
        ) : (
          <p className="adm-muted">{t("admin.related.none")}</p>
        )}
        <div className="adm-form-actions">
          <Button
            size="sm"
            variant="ghost"
            icon={Plus}
            onClick={() => {
              setDrafts((current) => [
                ...current,
                {
                  key: nextKey,
                  handle: "",
                  keywords: keywordsText(saved.defaultKeywords),
                },
              ])
              setNextKey(nextKey + 1)
            }}
          >
            {t("admin.related.add")}
          </Button>
          <Button
            type="submit"
            size="sm"
            icon={Save}
            busy={busy}
            disabled={!dirty || busy}
          >
            {t("admin.action.save")}
          </Button>
        </div>
      </form>
    </Panel>
  )
}
