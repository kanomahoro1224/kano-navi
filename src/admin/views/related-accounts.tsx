import { useEffect, useState, type SubmitEvent } from "react"
import { Newspaper, Plus, Save, Trash2, UsersRound } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import type { ViewProps } from "@/admin"
import { japanStamp } from "@/admin/format"
import type { XRelatedAccount, XRelatedAccounts } from "@/admin/types"
import {
  Badge,
  Button,
  Field,
  FieldGroup,
  IconButton,
  Loading,
  Panel,
  TagInput,
} from "@/admin/ui"
import { useAdmin } from "@/admin/use-admin-data"

/** The server's bounds of one account's keyword rules. */
const keywordLimit = 30
const keywordLength = 100

interface Draft {
  key: number
  handle: string
  keywords: string[]
}

const draftsOf = (accounts: XRelatedAccount[]): Draft[] =>
  accounts.map((account, key) => ({
    key,
    handle: account.handle,
    keywords: account.keywords,
  }))

const accountsOf = (drafts: Draft[]): XRelatedAccount[] =>
  drafts.map((draft) => ({
    handle: draft.handle.trim().replace(/^@/u, ""),
    keywords: draft.keywords,
  }))

/**
 * The related X accounts on their timer's Sources page: accounts such as the
 * agency whose own timelines each x_related run reads in full, through the
 * FxTwitter timeline only, into External posts. A status whose text contains
 * one of the account's keyword rules, entered as chips, is promoted onto the
 * board. The page holds the one read of the accounts, which the manual fetch
 * beside it shares.
 */
export function RelatedAccountsPanel({
  related,
  onSaved,
  navigate,
}: {
  related: XRelatedAccounts | undefined
  onSaved: (related: XRelatedAccounts) => void
  navigate: ViewProps["navigate"]
}) {
  const { t } = useAppSettings()
  const { api, act } = useAdmin()
  const [drafts, setDrafts] = useState<Draft[]>([])
  const [nextKey, setNextKey] = useState(0)
  const [busy, setBusy] = useState(false)
  const saved = related
  // A reread after a run, which only changes the read status, keeps the
  // edits; a changed account list replaces them.
  const savedKey = saved ? JSON.stringify(saved.accounts) : ""
  useEffect(() => {
    if (!saved) return
    setDrafts(draftsOf(saved.accounts))
    setNextKey(saved.accounts.length)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [savedKey])
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
    if (next) onSaved(next)
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
                    <FieldGroup label={t("admin.related.keywords")}>
                      <TagInput
                        value={draft.keywords}
                        onChange={(keywords) => set(draft.key, { keywords })}
                        label={t("admin.related.keywordsFor", {
                          handle: draft.handle || "…",
                        })}
                        placeholder={t("admin.models.keywordsPlaceholder")}
                        max={keywordLimit}
                        maxLength={keywordLength}
                      />
                    </FieldGroup>
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
                  keywords: saved.defaultKeywords,
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
