# Poster Popup System Specification

## ADDED Requirements

### Requirement: OSS Configuration Stored In Option Table
The system SHALL store Aliyun OSS credentials and bucket info in the
existing option table so they can be edited at runtime via the admin
Setting page, with the secret obscured on read.

#### Scenario: Admin sets OSS credentials via Setting page
- **GIVEN** a root admin opens the Setting Dashboard "海报弹窗" tab
- **WHEN** the admin enters OSSAccessKeyId, OSSAccessKeySecret, OSSEndpoint, OSSBucket and saves
- **THEN** all four values SHALL be persisted to the option table
- **AND** the runtime `common.OSSAccessKeyId / OSSAccessKeySecret / OSSEndpoint / OSSBucket` SHALL be updated immediately without restart

#### Scenario: Secret is obscured when admin reads option list
- **GIVEN** OSSAccessKeySecret is non-empty in the option table
- **WHEN** a root admin GETs the option list (existing endpoint that returns all options)
- **THEN** OSSAccessKeySecret in the response SHALL be replaced with `***` (placeholder string)
- **AND** the actual secret SHALL never leave the backend

#### Scenario: Saving with placeholder secret keeps original
- **GIVEN** the Setting page Secret field still shows the placeholder `***` (admin did not retype it)
- **WHEN** the admin saves the form
- **THEN** the system SHALL detect the placeholder and NOT overwrite the stored secret with `***`
- **AND** the original secret SHALL remain intact

#### Scenario: Empty string update is also a no-op for OSSAccessKeySecret
- **GIVEN** a stored OSSAccessKeySecret with a real value
- **WHEN** the option update receives `OSSAccessKeySecret = ""` (empty string;
  may happen if the admin clears the password input intending only "do not modify")
- **THEN** the system SHALL NOT overwrite the stored secret with empty
- **AND** the original secret SHALL remain intact
- **AND** the operator-facing way to truly clear OSS configuration is to
  toggle EnablePoster off (the secret can be reset via DB if ever needed)

#### Scenario: Placeholder collision is documented
- **GIVEN** the placeholder string is exactly the 3-character literal `***`
- **AND** an Aliyun OSS AccessKeySecret is normally a 30+ character base64-like string
- **WHEN** documenting OSS configuration for operators
- **THEN** the operations document SHALL note that admins MUST NOT set the literal `***` as their AccessKeySecret (to avoid the false-detection edge case)
- **AND** the system SHALL accept any other string as a real value, including strings that contain `***` as a substring (e.g., `abc***def`)

### Requirement: Poster Image Upload To OSS
The system SHALL provide a root root-admin-only API that uploads an image
file to the configured Aliyun OSS bucket and returns its public URL.

#### Scenario: Admin uploads valid image
- **GIVEN** OSS credentials are fully configured
- **AND** a root admin posts a multipart file `image/jpeg` of size 1 MB
- **WHEN** the admin calls `POST /api/option/poster/upload`
- **THEN** the system SHALL upload the file to the bucket with object key `posters/poster_<uuid><ext>`
- **AND** the response SHALL be `{success: true, data: {url: "<public_url>"}}`
- **AND** the request SHALL pass the existing UploadRateLimit middleware

#### Scenario: Upload rejects oversized file
- **GIVEN** the admin uploads a file > 5 MB
- **WHEN** the request is processed
- **THEN** the response SHALL be a 4xx with message indicating size limit
- **AND** no OSS call SHALL be made

#### Scenario: Upload rejects unsupported mime type
- **GIVEN** the admin uploads a file whose Content-Type is not in the whitelist (`image/jpeg / image/png / image/webp / image/gif`)
- **WHEN** the request is processed
- **THEN** the response SHALL be a 4xx with message indicating type restriction
- **AND** no OSS call SHALL be made

#### Scenario: Upload fails when OSS not configured
- **GIVEN** OSSAccessKeyId or OSSAccessKeySecret or OSSEndpoint or OSSBucket is empty
- **WHEN** a root admin attempts to upload
- **THEN** the response SHALL be a 4xx with message asking to configure OSS first
- **AND** no upload SHALL be attempted

#### Scenario: Uploaded object is set to public-read ACL
- **GIVEN** a bucket whose default ACL may be private
- **AND** a successful PutObject call from the upload handler
- **WHEN** the SDK PutObject is invoked
- **THEN** it SHALL include the option `oss.ObjectACL(oss.ACLPublicRead)` so the uploaded object is publicly readable regardless of the bucket-level default
- **AND** the front-end `<img>` tag fetched anonymously SHALL succeed for the returned URL
- **AND** if the RAM credential lacks `oss:PutObjectAcl` permission, the upload SHALL still succeed when the bucket ACL is already public-read; otherwise the SDK error message SHALL surface to the admin

#### Scenario: Successful upload does NOT auto-update PosterImageUrl
- **GIVEN** a successful upload returning a public URL
- **WHEN** the response is delivered to the frontend
- **THEN** the OSS URL SHALL appear in the frontend `PosterImageUrl` input field but the option table value SHALL remain unchanged until the admin clicks Save
- **AND** the admin can still preview, modify, or discard the URL before persisting

### Requirement: Poster Configuration Endpoint
The system SHALL provide a public endpoint that returns the current
poster configuration (3 fields) for the homepage popup.

#### Scenario: Public client fetches poster config
- **GIVEN** an authenticated or anonymous user opens the homepage
- **WHEN** the frontend calls `GET /api/poster`
- **THEN** the response SHALL be `{success: true, data: {enabled: <bool>, image_url: <string>, click_url: <string>}}`
- **AND** the endpoint SHALL NOT require admin permission

#### Scenario: Disabled poster returns enabled=false
- **GIVEN** EnablePoster is false
- **WHEN** the public endpoint is called
- **THEN** the response SHALL be `{enabled: false, image_url: <stored_value>, click_url: <stored_value>}`
- **AND** the frontend SHALL NOT pop the modal even if image_url is non-empty

### Requirement: Homepage Popup Priority And Frequency
The frontend SHALL prefer the poster popup over the existing
announcement popup, and SHALL pop each poster at most once per day.

#### Scenario: Poster takes priority over announcement
- **GIVEN** the public `/api/poster` returns `{enabled: true, image_url: "https://...x.jpg", click_url: ""}`
- **AND** the existing `/api/notice` returns a non-empty announcement string
- **WHEN** the user opens the homepage
- **THEN** the system SHALL show the PosterModal
- **AND** SHALL NOT show the existing NoticeModal in the same session

#### Scenario: Falls back to announcement when poster disabled or empty
- **GIVEN** EnablePoster is false OR image_url is empty
- **WHEN** the homepage mounts
- **THEN** the system SHALL bypass the poster modal entirely
- **AND** SHALL run the existing `checkNoticeAndShow` flow with no behavioral change

#### Scenario: Poster shown at most once per day per image_url
- **GIVEN** the user has previously closed a poster with image_url=X today (same calendar date)
- **WHEN** the user revisits the homepage
- **THEN** the system SHALL NOT pop the poster again today
- **AND** the localStorage key `poster_seen_<md5(X).slice(0,8)>_<YYYYMMDD>` SHALL still exist

#### Scenario: Different poster image triggers new popup
- **GIVEN** a root admin updates PosterImageUrl from X to Y (different URL)
- **AND** the user has the localStorage key from yesterday's poster X
- **WHEN** the user visits the homepage
- **THEN** the system SHALL pop the new poster Y
- **AND** the localStorage key check uses the new hash, finds nothing, and proceeds

#### Scenario: Same-day poster swap also triggers new popup
- **GIVEN** earlier today the user closed a poster with image_url=X (localStorage key for X+today exists)
- **AND** a root admin then updates PosterImageUrl from X to Y on the same calendar day
- **WHEN** the user revisits the homepage later that same day
- **THEN** the system SHALL pop the new poster Y
- **AND** the lookup uses key `poster_seen_<md5(Y).slice(0,8)>_<TODAY>` which does not exist, so the popup proceeds
- **AND** the user closing it writes the new key without affecting X's key

#### Scenario: Closing poster does not interfere with announcement
- **GIVEN** the user closes the poster modal
- **WHEN** the same session continues
- **THEN** the announcement modal SHALL NOT appear (priority is for the same homepage mount)
- **AND** next day's homepage mount with same poster URL falls back to announcement (poster not shown again)

### Requirement: Poster Click-Through Link
The poster SHALL be clickable when click_url is non-empty, opening
the URL in a new tab.

#### Scenario: Poster with click_url is wrapped in anchor
- **GIVEN** click_url is `https://mp.weixin.qq.com/article-abc`
- **WHEN** the PosterModal renders
- **THEN** the image SHALL be wrapped in `<a href="https://mp.weixin.qq.com/article-abc" target="_blank" rel="noopener noreferrer">`
- **AND** clicking the image SHALL open the URL in a new tab

#### Scenario: Poster without click_url is non-interactive
- **GIVEN** click_url is empty string
- **WHEN** the PosterModal renders
- **THEN** the image SHALL NOT be wrapped in an anchor
- **AND** the image SHALL be displayed without click handler

### Requirement: Poster Image Load Failure Is Silent
The PosterModal SHALL handle broken image URLs gracefully without
breaking the modal close flow.

#### Scenario: Broken image URL still allows close
- **GIVEN** PosterImageUrl points to a non-existent OSS object
- **WHEN** the modal renders
- **THEN** the `<img onError>` handler SHALL hide the image (or show a placeholder)
- **AND** the close button SHALL still be usable
- **AND** localStorage SHALL still record this poster was "shown" so it doesn't loop

### Requirement: Existing Announcement Mechanism Unchanged
The existing system announcement mechanism SHALL remain functionally unchanged: OptionMap["Notice"], /api/notice endpoint, NoticeModal component, and localStorage.notice_close_date all continue to behave exactly as before this change.

#### Scenario: When poster is not configured, announcement behaves as before
- **GIVEN** EnablePoster=false (or never configured)
- **WHEN** a user visits the homepage
- **THEN** the homepage behavior SHALL be byte-identical to pre-change behavior:
  - calls `/api/notice`
  - checks `localStorage.notice_close_date`
  - pops `NoticeModal` if applicable

#### Scenario: Backend announcement endpoint is not modified
- **GIVEN** any client calls `GET /api/notice`
- **WHEN** the response is built
- **THEN** the response SHALL be identical to pre-change behavior (no schema changes)

### Requirement: Configuration Flags
The system SHALL expose configuration flags so the poster behavior
can be tuned without code changes, all routed through OptionMap.

#### Scenario: Defaults are safe
- **GIVEN** a fresh install with no admin-set values
- **WHEN** the OptionMap initializes
- **THEN** all 7 new keys SHALL default to:
  - OSSAccessKeyId / OSSAccessKeySecret / OSSEndpoint / OSSBucket: empty string
  - PosterImageUrl / PosterClickUrl: empty string
  - EnablePoster: false (BOOL string `"false"`)
- **AND** no poster modal SHALL ever appear in this state

#### Scenario: Hot reload via existing option update flow
- **GIVEN** a root admin updates EnablePoster from false to true via the existing option PUT endpoint
- **WHEN** the next user fetches `/api/poster`
- **THEN** the response SHALL reflect the new value without a server restart
- **AND** the same applies to the other 6 keys
