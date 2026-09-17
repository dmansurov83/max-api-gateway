"""Notification service for MAX Gateway."""

from __future__ import annotations

import logging
from pathlib import Path

import aiohttp

from homeassistant.components.notify import ATTR_IMAGE, ATTR_TITLE, BaseNotificationService
from homeassistant.const import CONF_API_KEY, CONF_URL
from homeassistant.core import HomeAssistant
from homeassistant.helpers.typing import ConfigType, DiscoveryInfoType

_LOGGER = logging.getLogger(__name__)

CONF_CHAT_ID = "chat_id"
_IMAGE_MIME = {
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".png": "image/png",
    ".gif": "image/gif",
    ".webp": "image/webp",
}


async def async_get_service(
    hass: HomeAssistant,
    config: ConfigType,
    discovery_info: DiscoveryInfoType | None = None,
) -> BaseNotificationService | None:
    """Get the notification service."""
    return MAXNotify(config)


class MAXNotify(BaseNotificationService):
    """Send notifications via the MAX Gateway."""

    def __init__(self, config: ConfigType) -> None:
        self._base_url = config[CONF_URL].rstrip("/")
        self._send_url = f"{self._base_url}/send"
        self._upload_url = f"{self._base_url}/upload"
        self._api_key = config.get(CONF_API_KEY, "")
        self._chat_id = config.get(CONF_CHAT_ID)

    @property
    def _headers(self) -> dict:
        if self._api_key:
            return {"Authorization": self._api_key}
        return {}

    def _build_payload(self, message: str, **kwargs) -> dict:
        text = message
        title = kwargs.get(ATTR_TITLE)
        if title:
            text = f"{title}\n{text}"
        payload = {"text": text}
        if self._chat_id is not None:
            payload["chat_id"] = self._chat_id
        return payload

    async def async_send_message(self, message: str, **kwargs) -> None:
        """Send a message (and optional image) to the configured MAX chat."""
        image = kwargs.get(ATTR_IMAGE)
        if image:
            await self._send_with_image(message, kwargs, image)
            return
        await self._send_text(message, kwargs)

    async def _send_text(self, message: str, kwargs: dict) -> None:
        payload = self._build_payload(message, kwargs)
        try:
            async with aiohttp.ClientSession() as session:
                async with session.post(
                    self._send_url, json=payload, headers=self._headers, timeout=10
                ) as resp:
                    text = await resp.text()
                    if resp.status != 200:
                        _LOGGER.error("MAX send failed: %s %s", resp.status, text)
        except aiohttp.ClientError as err:
            _LOGGER.error("MAX send request error: %s", err)

    async def _send_with_image(self, message: str, kwargs: dict, image: str) -> None:
        if image.startswith(("http://", "https://")):
            filename, content_type, data = await self._fetch_image(image)
            if data is None:
                return
        else:
            path = Path(image)
            filename = path.name
            content_type = _IMAGE_MIME.get(path.suffix.lower(), "application/octet-stream")
            try:
                data = path.read_bytes()
            except OSError as err:
                _LOGGER.error("MAX image read error: %s", err)
                return

        try:
            async with aiohttp.ClientSession() as session:
                form = aiohttp.FormData()
                if self._chat_id is not None:
                    form.add_field("chat_id", str(self._chat_id))
                form.add_field("text", self._build_payload(message, kwargs)["text"])
                form.add_field(
                    "file",
                    data,
                    filename=filename,
                    content_type=content_type,
                )
                async with session.post(
                    self._upload_url, data=form, headers=self._headers, timeout=30
                ) as resp:
                    text = await resp.text()
                    if resp.status != 200:
                        _LOGGER.error("MAX upload failed: %s %s", resp.status, text)
        except aiohttp.ClientError as err:
            _LOGGER.error("MAX upload request error: %s", err)

    async def _fetch_image(self, url: str) -> tuple[str, str, bytes | None]:
        """Download an image from a URL and return (filename, content_type, data)."""
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(url, timeout=30) as resp:
                    if resp.status != 200:
                        _LOGGER.error(
                            "MAX image fetch failed: %s", resp.status
                        )
                        return "", "", None
                    content_type = resp.headers.get("Content-Type", "application/octet-stream")
                    filename = Path(url.split("?")[0]).name or "image"
                    if "." not in filename:
                        filename = _filename_from_content_type(content_type)
                    data = await resp.read()
                    return filename, content_type, data
        except aiohttp.ClientError as err:
            _LOGGER.error("MAX image fetch error: %s", err)
            return "", "", None


def _filename_from_content_type(content_type: str) -> str:
    """Pick a sensible filename for a content type, e.g. image/jpeg -> image.jpg."""
    ext = {
        "image/jpeg": ".jpg",
        "image/png": ".png",
        "image/gif": ".gif",
        "image/webp": ".webp",
    }.get(content_type.split(";")[0].strip().lower())
    return f"image{ext}" if ext else "image.jpg"