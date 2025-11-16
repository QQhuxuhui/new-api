# Change: Add Real-Time Channel Concurrency Information Display

## Why
Administrators have configured concurrency limits for channel keys but cannot see the current concurrent request count in the channel management interface. This creates a blind spot - admins cannot monitor whether keys are approaching their limits, diagnose performance issues, or validate that concurrency controls are working as expected. Real-time visibility is essential for capacity planning and operational debugging.

## What Changes
- Add real-time concurrent request count display to channel management UI
- Extend channel API to return current concurrency metrics per key
- Display concurrency information for both single-key and multi-key channels
- Show visual indicators (usage percentage, color coding) for quick status assessment
- Update channel list table to show aggregate concurrency status

## Impact
- Affected specs: `channel-management`
- Affected code:
  - Backend: `controller/channel.go`, `service/concurrency.go`, `model/channel.go`
  - Frontend: `web/src/components/table/channels/`, `web/src/hooks/channels/useChannelsData.jsx`
- **Non-breaking**: Adds new optional fields to API responses
