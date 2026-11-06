package schedule

// These strings define model behaviour and are copied verbatim from
// server/schedule-extractor.ts. extractorVersion is part of stored extraction
// identities, so changing it reprocesses every known post.
const (
	extractorVersion     = "openai-schedule-v6"
	defaultOpenAIBaseURL = "https://api.openai.com/v1"

	protocolResponses = "openai-responses"
	protocolChat      = "openai-chat-completions"
	protocolAnthropic = "anthropic-messages"

	// routeMessage and routeBoard record a post's detection path; both paths
	// draw their models from the extraction queue stored as routeQueue.
	routeMessage = "schedule_message"
	routeBoard   = "schedule_board"
	routeQueue   = "schedule"
	// routeVerify is the separate schedule-image check queue; it never falls
	// back to the extraction queue.
	routeVerify = "verify"
	// routeTranslate is the separate translation queue; it never falls back
	// to the schedule queue.
	routeTranslate = "translate"

	extractionToolName   = "kano_schedule_extraction"
	verificationToolName = "kano_schedule_asset_verification"
	translationToolName  = "kano_text_translation"
	eventTagToolName     = "kano_event_tag"

	// eventTagInstructions classifies one YouTube or Bilibili title for the
	// calendar and the video archive. It is a short-text judgement like
	// translation, so it shares that queue rather than adding a queue and model
	// settings of its own. A video keeps only game, talk and song; live and video
	// name a form the video already has.
	eventTagInstructions = "Classify one video title by Kano Mahoro (鹿乃まほろ), a Japanese virtual singer, for a fan calendar and video archive. " + eventTagRule + " Return null when the title does not show what kind of activity it is; never guess from the channel name alone."

	// eventTagRule defines the five labels for both the title inference and
	// the extraction prompt, so a stream or premiere gets the same label
	// whichever path judges it. game, talk and song name the content; live and
	// video name only the form and are used when no content label applies.
	eventTagRule = "Choose by content first: game for gameplay, talk for a chat or talk stream, and song for singing, including a singing stream, a song cover, an original song, a music video and a song premiere. Use live only for a broadcast or stream that is none of these or whose content is not shown, and video only for an uploaded video or premiere that is none of these or whose content is not shown. When an entry mixes activities, use the one it leads with; when none leads, use live for a stream and video for an upload or premiere."

	eventTagSchema rawJSON = `{"type":"object","additionalProperties":false,"properties":{"tag":{"anyOf":[{"type":"string","enum":["live","game","talk","song","video"]},{"type":"null"}]}},"required":["tag"]}`

	translationInstructions = "Translate one X post or YouTube video title by Kano Mahoro (鹿乃まほろ), a Japanese virtual singer, for readers of an unofficial fan status board. Set sourceLanguage to the language of the text: ja, en, zh, ko or other, or none when it has nothing to translate (only links, hashtags, @mentions, emoji or kaomoji). Translate the whole text into natural English (english) and Simplified Chinese (simplifiedChinese). Keep URLs, hashtags, @mentions, emoji, kaomoji, bracketed channel tags and line breaks as written. Use the usual romanization or official names of people, groups and games, and keep a song title in its original wording unless it has an official translation. Each translation field is the complete text to display, not a list of changes: when a song title or name stays unchanged, include that original wording in the non-empty result. For sourceLanguage ja, ko or other, both english and simplifiedChinese must be non-empty, even when the whole title stays unchanged. Do not add explanations, quotation marks or notes. Return an empty string only for english when sourceLanguage is en, only for simplifiedChinese when sourceLanguage is zh, and for both when sourceLanguage is none."

	translationSchema rawJSON = `{"type":"object","additionalProperties":false,"properties":{"sourceLanguage":{"type":"string","enum":["ja","en","zh","ko","other","none"]},"english":{"type":"string"},"simplifiedChinese":{"type":"string"}},"required":["sourceLanguage","english","simplifiedChinese"]}`

	scheduleInstructions = "First classify whether this single Japanese X post is a concrete schedule notice. Return classification=not_schedule when it is ordinary conversation or has no concrete schedule, and classification=uncertain when the evidence is ambiguous. For classification=schedule, extract only entries explicitly supported by the post text or images. Interpret relative dates from the post publication time in Asia/Tokyo. Never invent a time. Use null time and timePrecision unknown when the date is known but the time is not. " + scheduleDateInstruction + " " + scheduleTypeInstruction + " " + scheduleSaleInstruction + " Set action=cancel only when the post is evidence that an existing schedule may have been cancelled; this is a judgement for review, never a deletion or definitive cancellation. Put the reason in the root reason field and cite the post wording in evidence. For cancellation, events should identify the affected date/title/time when possible. " + scheduleLanguageInstruction + " Return an empty events array for not_schedule. " + scheduleTagInstruction

	// scheduleDateInstruction matches the stored event check: an entry without
	// a calendar day fails the whole extraction, so the model leaves it out.
	scheduleDateInstruction = "Give date as the Asia/Tokyo calendar date YYYY-MM-DD, and time and endTime as 24-hour HH:MM. An entry needs its calendar day: when the post gives only a month, a season or a vague period such as 4月 or 春頃, leave that entry out, and return classification=not_schedule when no entry remains."

	// scheduleTypeInstruction names what each eventType code covers, so a
	// release (a work becoming available) and a sale (a period to buy or
	// apply) are told apart.
	scheduleTypeInstruction = "Set eventType to stream for a live stream, member for a members-only stream, appearance for a performance or a guest appearance, release for the date a work becomes available (a song, music video, premiere, album or CD), sale for a period to buy, pre-order, order or apply for goods, media or tickets (a ticket lottery included), and event for any other entry."

	// scheduleSaleInstruction asks for one entry per sale period. A post that
	// names only the deadline stores a period whose opening is not known: its
	// date and endDate are both the deadline and it has no start time.
	scheduleSaleInstruction = "Give one sale entry per sale period, never separate entries for its opening and its deadline. Set date and time to when the period opens (本日から or 今日から means the post date) and endDate and endTime to when it closes. When the post gives only the deadline, set date and endDate to the deadline date, time to null and endTime to the deadline time or null. Use null endDate when the close is not given and for every entry that is not a sale. A shipping or delivery date is not an entry."

	// scheduleTagInstruction describes the optional calendar label. The label
	// describes one calendar entry, so it belongs to the event rather than to
	// the video a reader may open from it.
	scheduleTagInstruction = "Set tag only when the post or image itself shows what kind of activity the entry is. " + eventTagRule + " When the entry does not say, use null; never guess from the channel or a person's name."

	// English is the base language of stored text; the board translates
	// status codes and shows titles in their original wording.
	scheduleLanguageInstruction = "Keep each title and the cited evidence in the post's original wording. Write detail and reason in English, and give status as one short lowercase English word such as scheduled, tentative or released."

	scheduleAssetVerificationInstructions = "Inspect the attached image only and decide whether it is a weekly or multi-day schedule board for the creator. Return schedule only when the image itself visibly contains a structured calendar or schedule table with days or dates and planned items. Return not_schedule for a stream thumbnail, promotional art, character art, a single video card, or any ordinary post image. Return uncertain when the image is unreadable or the evidence is insufficient. Do not infer a schedule from the source post wording. Cite visible image evidence briefly."

	// JSON.stringify(scheduleSchema) and
	// JSON.stringify(scheduleAssetVerificationSchema), byte-exact.
	scheduleSchema rawJSON = `{"type":"object","additionalProperties":false,"properties":{"classification":{"type":"string","enum":["schedule","not_schedule","uncertain"]},"events":{"type":"array","items":{"type":"object","additionalProperties":false,"properties":{"title":{"type":"string"},"detail":{"anyOf":[{"type":"string"},{"type":"null"}]},"date":{"type":"string"},"time":{"anyOf":[{"type":"string"},{"type":"null"}]},"endTime":{"anyOf":[{"type":"string"},{"type":"null"}]},"endDate":{"anyOf":[{"type":"string"},{"type":"null"}]},"timePrecision":{"type":"string","enum":["exact","approximate","unknown"]},"status":{"type":"string"},"eventType":{"type":"string","enum":["event","stream","member","release","appearance","sale"]},"tag":{"anyOf":[{"type":"string","enum":["live","game","talk","song","video"]},{"type":"null"}]},"url":{"anyOf":[{"type":"string"},{"type":"null"}]},"confidence":{"type":"number","minimum":0,"maximum":1},"evidence":{"type":"string"}},"required":["title","detail","date","time","endTime","endDate","timePrecision","status","eventType","tag","url","confidence","evidence"]}},"confidence":{"type":"number","minimum":0,"maximum":1},"evidence":{"type":"string"},"reason":{"anyOf":[{"type":"string"},{"type":"null"}]},"action":{"type":"string","enum":["add","update","cancel","none"]}},"required":["classification","events","confidence","evidence","reason","action"]}`

	scheduleAssetVerificationSchema rawJSON = `{"type":"object","additionalProperties":false,"properties":{"classification":{"type":"string","enum":["schedule","not_schedule","uncertain"]},"confidence":{"type":"number","minimum":0,"maximum":1},"evidence":{"type":"string"},"reason":{"type":"string"}},"required":["classification","confidence","evidence","reason"]}`
)

var defaultScheduleKeywords = []string{
	"schedule",
	"スケジュール",
	"配信予定",
	"週間予定",
	"今週の予定",
	"予定",
}
