-- playlistmaker-history-version: 10
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
local user_paused = mp.get_property_native("pause") or false
local hold = manifest.statusPath ~= nil
local untracked = false
local imposed_pause = nil
local intent_sequence = 0
local heartbeat = nil
local heartbeat_seen = mp.get_time()
local status_revision = -1
local status_message = "Waiting for tracking helper"
local status_state = nil
local health_failed = false
local emit_intent
local enforce_hold

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

local function emit_session_event(name, position, reason, completed)
    event_sequence = event_sequence + 1
    append_json(config.event_path, {
        eventId = manifest.sessionId .. ":" .. tostring(event_sequence),
        event = name,
        sessionId = manifest.sessionId,
        occurrenceId = active and active.play_id,
        paused = user_paused,
        intentSequence = intent_sequence,
        eventAtUtc = utc_now(),
        playlistPosition = position,
        endReason = reason,
        completed = completed,
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
        sessionId = manifest.sessionId,
        occurrenceId = active and active.play_id,
        paused = user_paused,
        intentSequence = intent_sequence,
        eventAtUtc = utc_now(),
        sessionId = manifest.sessionId,
        entryId = entry.entryId,
        playId = active and active.play_id,
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
    return counted
end

local function media_position()
    local pos = mp.get_property_number("time-pos", nil)
    if not pos then return nil end
    local duration, raw, start = playback_duration()
    if start and start > 0 and raw and raw > start then pos = pos - start end
    return math.floor(math.max(0, pos) * 1000)
end

emit_intent = function(name)
    if not active then return end
    intent_sequence = intent_sequence + 1
    event_sequence = event_sequence + 1
    append_json(config.event_path, {
        eventId = manifest.sessionId .. ":" .. tostring(event_sequence),
        sessionId = manifest.sessionId, occurrenceId = active.play_id,
        event = name or "intent", eventAtUtc = utc_now(),
        playlistPosition = active.entry.playlistPosition,
        paused = user_paused, positionMs = media_position(),
        intentSequence = intent_sequence, statusRevision = status_revision,
    })
end

enforce_hold = function()
    local desired = user_paused or (hold and not untracked)
    if mp.get_property_native("pause") ~= desired then
        imposed_pause = desired
        mp.set_property_native("pause", desired)
    end
end

local restart_requested = false
local function user_intent(paused)
    user_paused = paused
    if not paused and health_failed and manifest.helperExecutable and not restart_requested then
        restart_requested = true
        mp.command_native_async({name="subprocess", args={manifest.helperExecutable,"--track-session",config.manifest_path}, playback_only=false}, function() restart_requested=false end)
    end
    if not paused and manifest.statusPath and not untracked then hold = true end
    emit_intent("intent")
    enforce_hold()
end

mp.observe_property("pause", "bool", function(_, paused)
    if imposed_pause ~= nil and paused == imposed_pause then imposed_pause = nil; return end
    if not active then return end
    user_intent(paused)
end)
mp.add_forced_key_binding("SPACE", "tracking-toggle", function() if hold then user_intent(false) else user_intent(not user_paused) end end)
mp.add_forced_key_binding("p", "tracking-toggle-p", function() user_intent(not user_paused) end)
mp.add_forced_key_binding("PLAY", "tracking-play", function() user_intent(false) end)
mp.add_forced_key_binding("PAUSE", "tracking-pause", function() user_intent(true) end)
mp.register_script_message("playlistmaker-retry", function() user_intent(false) end)
mp.register_script_message("playlistmaker-pause", function() user_intent(true) end)
local function continue_untracked()
    emit_intent("untracked")
    untracked, hold, user_paused = true, false, false
    enforce_hold()
    mp.osd_message("Continuing without tracking", 5)
end
mp.add_forced_key_binding("Ctrl+Shift+u", "continue-without-tracking", continue_untracked)
mp.register_script_message("playlistmaker-untracked", continue_untracked)

local function start_play(name)
    local position = mp.get_property_number("playlist-pos", -1)
    local entry = manifest.entries[position + 1]
    if not entry then return end
    terminal_entries[entry.entryId] = nil
    local duration, raw, start = playback_duration()
    active = {entry = entry, watched_seconds = 0, last_tick = mp.get_time(), duration = duration, raw = raw, start = start}
    emit_session_event(name, position, nil)
    active.play_id = manifest.sessionId .. ":" .. tostring(event_sequence)
    if manifest.statusPath and not untracked then hold = true end
    emit_intent("intent")
    enforce_hold()
    write_history("started", entry, {durationSeconds = duration, rawDurationSeconds = raw, demuxerStartSeconds = start})
end

mp.register_event("file-loaded", function()
    start_play("file-loaded")
end)

-- Single-file loops seek instead of loading a file. Recognize the end-to-start
-- wrap so the tracking player and history both get a separate playthrough.
mp.observe_property("time-pos", "number", function(_, position)
    if not active or not position then return end
    local previous = active.position
    active.position = position
    if user_paused then emit_intent("intent"); return end
    local duration = active.duration
    local looping = mp.get_property("loop-file", "no") ~= "no"
    if not looping or not previous or not duration or duration <= 0 then return end
    local margin = math.min(1, duration / 4)
    if previous >= duration - margin and position <= margin and previous > position then
        finish_active("eof")
        start_play("playback-repeat")
        active.position = position
    end
end)
mp.add_periodic_timer(0.25, function()
    if manifest.statusPath and not untracked then
        local file = io.open(manifest.statusPath, "r")
        local status = nil
        if file then status = utils.parse_json(file:read("*a")); file:close() end
        if status and status.sessionId == manifest.sessionId then
            if heartbeat ~= status.heartbeat then
                heartbeat = status.heartbeat
                heartbeat_seen = mp.get_time()
                health_failed = false
            end
            if active and status.occurrenceId == active.play_id and status.intentSequence == intent_sequence then
                hold = status.hold
                status_message = status.message
                status_state = status.state
                if status_revision ~= status.revision then
                    status_revision = status.revision
                    enforce_hold()
                    -- Acknowledgements do not change user intent sequence.
                    event_sequence = event_sequence + 1
                    append_json(config.event_path, {eventId=manifest.sessionId .. ":" .. event_sequence,
                        sessionId=manifest.sessionId, occurrenceId=active.play_id,
                        event="hold-ack", paused=mp.get_property_native("pause"), statusRevision=status_revision, eventAtUtc=utc_now()})
                end
            end
        end
        if mp.get_time() - heartbeat_seen > 15 then
            hold = true
            status_message = "Tracking helper unresponsive. Video held; restart helper or Ctrl+Shift+U to continue without tracking."
            if not health_failed then
                -- Not an intent: the helper never acknowledges it, so bumping the
                -- sequence would keep the hold after the heartbeat recovers.
                emit_session_event("health-failure", -1); health_failed = true
                if manifest.helperExecutable then
                    mp.command_native_async({name="subprocess",args={manifest.helperExecutable,"--tracking-notification"},playback_only=false},function() end)
                end
            end
        end
        enforce_hold()
        if hold or user_paused then
            mp.osd_message("PlaylistMaker: " .. (status_message or "Tracking held") .. "\nPlay: retry/resume | Ctrl+Shift+U: without tracking", 1)
        elseif status_state == "starting" then
            -- A late song start keeps playing briefly; the helper holds if it stays unconfirmed.
            mp.osd_message("PlaylistMaker: " .. (status_message or "Waiting for Spotify"), 1)
        end
    end
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
    local completed = finish_active(reason)
    emit_session_event("end-file", position, reason, completed)
end)

mp.register_event("shutdown", function()
    local completed = finish_active("quit")
    emit_session_event("shutdown", -1, "quit", completed)
    for _, entry in ipairs(manifest.entries) do
        if not terminal_entries[entry.entryId] then
            write_history("not_started", entry, {endReason = "mpv-shutdown-before-file-loaded", countedAsPlayed = false})
            terminal_entries[entry.entryId] = true
        end
    end
end)
