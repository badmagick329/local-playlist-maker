-- playlistmaker-history-version: 4
local mp = require("mp")
local options = require("mp.options")
local utils = require("mp.utils")

local config = {
    manifest_path = "",
    event_path = "",
    history_path = "",
    minimum_watched_percent = 50,
}

options.read_options(config, "playlistmaker_history")
if config.manifest_path == "" or config.event_path == "" then
    return
end

local manifest_file = io.open(config.manifest_path, "r")
if not manifest_file then
    mp.msg.error("PlaylistMaker: cannot read playback manifest")
    return
end
local manifest = utils.parse_json(manifest_file:read("*a"))
manifest_file:close()
if not manifest or not manifest.sessionId or not manifest.entries then
    mp.msg.error("PlaylistMaker: invalid playback manifest")
    return
end

local active = nil
local terminal_entries = {}
local event_sequence = 0

local function utc_now()
    return os.date("!%Y-%m-%dT%H:%M:%SZ")
end

local function append_json(path, value)
    local file, message = io.open(path, "a")
    if not file then
        mp.msg.error("PlaylistMaker: cannot write event: " .. (message or "unknown error"))
        return
    end
    file:write(utils.format_json(value) .. "\n")
    file:close()
end

local function emit_session_event(name, position, reason)
    event_sequence = event_sequence + 1
    append_json(config.event_path, {
        eventId = manifest.sessionId .. ":" .. tostring(event_sequence),
        event = name,
        eventAtUtc = utc_now(),
        playlistPosition = position,
        endReason = reason,
    })
end

local function write_history(name, entry, fields)
    if config.history_path == "" then
        return
    end
    local track = entry.track or {}
    local value = {
        schemaVersion = 3,
        event = name,
        eventAtUtc = utc_now(),
        sessionId = manifest.sessionId,
        entryId = entry.entryId,
        playlistPosition = entry.playlistPosition,
        playlistSize = #manifest.entries,
        selectionSource = "charm-tui",
        trackId = track.trackId,
        videoPath = entry.videoPath,
        audioPath = track.localAudioPath,
        artist = track.artist,
        title = track.title,
    }
    for key, item in pairs(fields or {}) do
        value[key] = item
    end
    append_json(config.history_path, value)
end

local function playback_duration()
    local raw = mp.get_property_number("duration", nil)
    local start = mp.get_property_number("demuxer-start-time", nil)
    local duration = raw
    if duration and start and start > 0 and duration > start then
        duration = duration - start
    end
    return duration, raw, start
end

local function finish_active(reason)
    if not active or terminal_entries[active.entry.entryId] then
        return
    end
    local now = mp.get_time()
    if not mp.get_property_native("pause") then
        active.watched_seconds = active.watched_seconds + math.max(0, now - active.last_tick)
    end
    local duration, raw, start = playback_duration()
    duration = duration or active.duration
    raw = raw or active.raw
    start = start or active.start
    local watched = active.watched_seconds
    if duration and duration > 0 then
        watched = math.max(0, math.min(duration, watched))
    end
    local percent = 0
    if duration and duration > 0 then
        percent = math.max(0, math.min(100, watched / duration * 100))
    end
    if reason == "eof" then percent = 100 end
    local name, counted = "skipped", false
    if reason == "eof" or percent >= 90 then
        name, counted = "completed", true
    elseif percent >= tonumber(config.minimum_watched_percent) then
        name, counted = "stopped", true
    end
    write_history(name, active.entry, {
        durationSeconds = duration,
        rawDurationSeconds = raw,
        demuxerStartSeconds = start,
        watchedSeconds = watched,
        watchedPercent = percent,
        finalPositionSeconds = mp.get_property_number("time-pos", nil),
        endReason = reason,
        countedAsPlayed = counted,
    })
    terminal_entries[active.entry.entryId] = true
    active = nil
end

mp.register_event("file-loaded", function()
    local position = mp.get_property_number("playlist-pos", -1)
    local entry = manifest.entries[position + 1]
    if not entry then return end
    terminal_entries[entry.entryId] = nil
    local duration, raw, start = playback_duration()
    active = {entry = entry, watched_seconds = 0, last_tick = mp.get_time(), duration = duration, raw = raw, start = start}
    emit_session_event("file-loaded", position, nil)
    write_history("started", entry, {durationSeconds = duration, rawDurationSeconds = raw, demuxerStartSeconds = start})
end)
mp.add_periodic_timer(0.25, function()
    if not active then return end
    local now = mp.get_time()
    if not mp.get_property_native("pause") then
        active.watched_seconds = active.watched_seconds + math.max(0, now - active.last_tick)
    end
    active.last_tick = now
end)

mp.register_event("end-file", function(event)
    local position = active and active.entry.playlistPosition or mp.get_property_number("playlist-pos", -1)
    local reason = event.reason or "unknown"
    finish_active(reason)
    emit_session_event("end-file", position, reason)
end)

mp.register_event("shutdown", function()
    finish_active("quit")
    emit_session_event("shutdown", -1, "quit")
    for _, entry in ipairs(manifest.entries) do
        if not terminal_entries[entry.entryId] then
            write_history("not_started", entry, {endReason = "mpv-shutdown-before-file-loaded", countedAsPlayed = false})
            terminal_entries[entry.entryId] = true
        end
    end
end)
