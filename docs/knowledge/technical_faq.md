# ZhiShi Platform - Technical FAQ

## System Requirements

### Mobile App
- **iOS**: iOS 14.0 or later, iPhone 8 or newer recommended
- **Android**: Android 8.0 or later, 3GB RAM minimum recommended
- **Storage**: At least 200MB free space for the app, plus space for downloaded content

### Web Platform
- **Browsers**: Chrome 90+, Firefox 88+, Safari 14+, Edge 90+
- **Screen resolution**: 1280x720 minimum, 1920x1080 recommended
- **Internet speed**: 5 Mbps minimum for smooth video playback, 10 Mbps recommended

---

## Common Technical Issues

### Video Won't Play
1. Check your internet connection speed
2. Try refreshing the page or restarting the app
3. Clear the app cache (Settings > Storage > Clear Cache)
4. Update the app to the latest version
5. If the issue persists, the video may have been removed by the creator

### App Crashes Frequently
1. Update to the latest version of the app
2. Clear the app cache and data
3. Restart your device
4. Ensure your device meets the minimum system requirements
5. Uninstall and reinstall the app if problems persist

### Upload Takes Too Long
1. Check your internet upload speed (at least 5 Mbps recommended)
2. Compress your video before uploading (use H.264 codec for best compatibility)
3. Try uploading during off-peak hours
4. Close other apps that may be using bandwidth
5. Try a WiFi connection instead of mobile data

### Video Quality Is Poor After Upload
- Upload in the highest resolution available (1080p or higher)
- Use the recommended codec: H.264 (MP4 container)
- Avoid multiple re-compressions before uploading
- The platform re-encodes videos for different quality levels; this is normal
- Allow up to 1 hour for HD quality processing to complete

### Search Returns No Results
1. Check for typos in your search query
2. Try different keywords or broader search terms
3. Use hashtag search (#tag) for more specific results
4. Filter by category to narrow down results
5. The search index updates every few minutes; very new content may not appear immediately

---

## API Information

### Platform API
- The ZhiShi platform provides a RESTful API for developers
- API base URL: /v1/
- Authentication: JWT (JSON Web Token) based
- Rate limiting: 100 requests per minute for standard users

### Available API Endpoints
- **Video operations**: Search, upload, stream, manage videos
- **User operations**: Registration, login, profile management
- **Interaction operations**: Likes, comments, follows, shares
- **AI operations**: Chat with AI assistant, knowledge base queries

---

## Data and Privacy

### Data Collection
- We collect usage data to improve the platform experience
- Video viewing history is used for content recommendations
- No personal data is shared with third parties without explicit consent

### Data Export
1. Go to Settings > Privacy > Download My Data
2. Request a data export (processed within 48 hours)
3. You'll receive a download link via email
4. Data includes: profile info, videos, comments, likes, and account activity

### Data Deletion
- Individual videos can be deleted at any time
- Comments can be deleted by the commenter or video owner
- Full account deletion is available (see Account FAQ section)
- All data is permanently removed 30 days after account deletion request

---

## Performance Optimization

### For Smooth Playback
- Enable "Auto Quality" in Settings > Video to let the app choose the best quality
- Close background apps to free up memory
- Use WiFi instead of mobile data for consistent speeds
- Enable "Preload Videos" to buffer upcoming content

### For Efficient Uploading
- Recommended video settings for upload:
  - Codec: H.264
  - Container: MP4
  - Resolution: 1080x1920 (vertical)
  - Frame rate: 30fps
  - Bitrate: 5-10 Mbps
  - Audio: AAC, 128kbps or higher

---

## Accessibility Features

### Available Features
- Subtitles/captions: Auto-generated and creator-provided
- Screen reader compatibility for navigation
- High contrast mode in Settings > Accessibility
- Adjustable playback speed (0.5x, 1x, 1.5x, 2x)
- Text size adjustment for comments and descriptions
- Reduced motion option for sensitive users

### How to Enable Accessibility Features
1. Go to Settings > Accessibility
2. Toggle the desired features on/off
3. Most features take effect immediately without restarting the app
