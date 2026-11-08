import { ShieldAlert, ShoppingBag } from "lucide-react"

import { useAppSettings } from "@/app-settings"
import { formatRelative, japanStamp } from "@/admin/format"
import type { Sources } from "@/admin/types"
import { Badge, Facts, Panel } from "@/admin/ui"
import { explainCode } from "@/admin/views/run-parts"

/** Whether a stored instant is still ahead. */
const future = (value: string | null | undefined) =>
  Boolean(value && Date.parse(value) > Date.now())

/**
 * The shop read's state on the Sources page, below its timer: the sale
 * entries it keeps, its last read and error, and the wait a 429 answer's
 * Retry-After set, during which no read requests the shop.
 */
export function ShopStatusPanel({ shop }: { shop: Sources["shop"] }) {
  const { t, locale } = useAppSettings()
  const status = shop.status
  const stamp = (value: string | null) =>
    value ? `${japanStamp(value)} · ${formatRelative(value, locale)}` : null
  return (
    <Panel
      level={3}
      icon={ShoppingBag}
      title={t("admin.shop.title")}
      description={t("admin.shop.description")}
      actions={
        <span className="adm-chip-row">
          {shop.skipped ? <Badge>{t("admin.overview.skipped")}</Badge> : null}
          {future(status.blockedUntil) ? (
            <Badge tone="danger" icon={ShieldAlert}>
              {t("admin.shop.blockedUntil", {
                time: japanStamp(status.blockedUntil ?? ""),
              })}
            </Badge>
          ) : null}
        </span>
      }
    >
      <Facts
        items={[
          [t("admin.shop.sales"), String(status.sales)],
          [t("admin.shop.lastAttempt"), stamp(status.lastAttemptAt)],
          [
            t("admin.shop.lastError"),
            status.lastError ? (
              <span title={status.lastError}>
                {explainCode(status.lastError, t) ?? status.lastError}
              </span>
            ) : null,
          ],
        ]}
      />
    </Panel>
  )
}
