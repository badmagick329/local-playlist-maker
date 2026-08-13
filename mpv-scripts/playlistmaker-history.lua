-- playlistmaker-history-version: 2
local mp = require("mp")
local options = require("mp.options")
local utils = require("mp.utils")

local config = {
    session_id = "",
    manifest_path = "",
    history_path = "",
    minimum_watched_percent = 50,
}

options.read_options(config, "playlistmaker_history")

if config.session_id == "" or config.manifest_path == "" or config.history_path == "" then
    return
end

local manifest_file = io.open(config.manifest_path, "r")
if not manifest_file then
    mp.msg.error("PlaylistMaker history: cannot read session manifest")
    return
end

local manifest_text = manifest_file:read("*a")
manifest_file:close()
manifest_text = manifest_text:gsub("^\239\187\191", "")
local manifest = utils.parse_json(manifest_text)
if not manifest or manifest.sessionId ~= config.session_id or not manifest.entries then
    mp.msg.error("PlaylistMaker history: invalid session manifest")
    return
end

local active = nil
local terminal_entries = {}

local function utc_now()
    return os.date("!%Y-%m-%dT%H:%M:%SZ")
end

local function write_event(event_name, entry, fields)
    local event = {
        schemaVersion = 2,
        event = event_name,
        eventAtUtc = utc_now(),
        sessionId = config.session_id,
        entryId = entry.entryId,
        playlistPosition = entry.playlistPosition,
        playlistSize = entry.playlistSize,
        selectionSource = entry.selectionSource,
        videoPath = entry.videoPath,
        audioPath = entry.audioPath,
        artist = entry.artist,
        title = entry.title,
    }

    for key, value in pairs(fields or {}) do
        event[key] = value
    end

    local history_file, error_message = io.open(config.history_path, "a")
    if not history_file then
        mp.msg.error("PlaylistMaker history: cannot write event: " .. (error_message or "unknown error"))
        return
    end

    history_file:write(utils.format_json(event) .. "\n")
    history_file:close()
end

local function watched_percent(watched_seconds, duration_seconds)
    if not duration_seconds or duration_seconds <= 0 then
        return 0
    end
    return math.max(0, math.min(100, watched_seconds / duration_seconds * 100))
end

local function finish_active(reason)
    if not active or terminal_entries[active.entry.entryId] then
        return
    end

    local now = mp.get_time()
    if not mp.get_property_native("pause") then
        active.watched_seconds = active.watched_seconds + math.max(0, now - active.last_tick)
    end

    local duration_seconds = mp.get_property_number("duration", active.duration_seconds)
    local final_position_seconds = mp.get_property_number("time-pos", nil)
    local normalized_watched_seconds = active.watched_seconds
    if duration_seconds and duration_seconds > 0 then
        normalized_watched_seconds = math.max(0, math.min(duration_seconds, normalized_watched_seconds))
    end
    local percent = watched_percent(normalized_watched_seconds, duration_seconds)
    local event_name
    local counted_as_played

    if reason == "eof" or percent >= 90 then
        event_name = "completed"
        counted_as_played = true
    elseif percent >= tonumber(config.minimum_watched_percent) then
        event_name = "stopped"
        counted_as_played = true
    else
        event_name = "skipped"
        counted_as_played = false
    end

    write_event(event_name, active.entry, {
        durationSeconds = duration_seconds,
        watchedSeconds = normalized_watched_seconds,
        watchedPercent = percent,
        finalPositionSeconds = final_position_seconds,
        endReason = reason,
        countedAsPlayed = counted_as_played,
    })
    terminal_entries[active.entry.entryId] = true
    active = nil
end

mp.register_event("file-loaded", function()
    local playlist_position = mp.get_property_number("playlist-pos", -1)
    local entry = manifest.entries[playlist_position + 1]
    if not entry or terminal_entries[entry.entryId] then
        return
    end

    active = {
        entry = entry,
        watched_seconds = 0,
        last_tick = mp.get_time(),
        duration_seconds = mp.get_property_number("duration", nil),
    }
    write_event("started", entry, {
        durationSeconds = active.duration_seconds,
    })
end)

mp.add_periodic_timer(0.25, function()
    if not active then
        return
    end

    local now = mp.get_time()
    if not mp.get_property_native("pause") then
        active.watched_seconds = active.watched_seconds + math.max(0, now - active.last_tick)
    end
    active.last_tick = now
end)

mp.register_event("end-file", function(event)
    finish_active(event.reason or "unknown")
end)

mp.register_event("shutdown", function()
    finish_active("quit")
    for _, entry in ipairs(manifest.entries) do
        if not terminal_entries[entry.entryId] then
            write_event("not_started", entry, {
                endReason = "mpv-shutdown-before-file-loaded",
                countedAsPlayed = false,
            })
            terminal_entries[entry.entryId] = true
        end
    end
end)
