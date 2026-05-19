use std::fs;
use zed_extension_api::{
    self as zed, settings::LspSettings, DownloadedFileType, LanguageServerId, Result,
};

const LSP_RELEASE_REPO: &str = "Max-Levitskiy/zed-extention-d2-viewer";

struct D2Extension {
    cached_binary_path: Option<String>,
}

impl zed::Extension for D2Extension {
    fn new() -> Self {
        Self { cached_binary_path: None }
    }

    fn language_server_command(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        if let Ok(settings) = LspSettings::for_worktree(language_server_id.as_ref(), worktree) {
            if let Some(binary) = settings.binary {
                if let Some(path) = binary.path {
                    return Ok(zed::Command {
                        command: path,
                        args: binary.arguments.unwrap_or_default(),
                        env: binary.env.map(|m| m.into_iter().collect()).unwrap_or_default(),
                    });
                }
            }
        }

        let path = self.resolve_binary(language_server_id)?;
        Ok(zed::Command { command: path, args: vec![], env: Default::default() })
    }
}

impl D2Extension {
    fn resolve_binary(&mut self, lsp_id: &LanguageServerId) -> Result<String> {
        if let Some(cached) = &self.cached_binary_path {
            if fs::metadata(cached).is_ok() {
                return Ok(cached.clone());
            }
        }

        zed::set_language_server_installation_status(
            lsp_id,
            &zed::LanguageServerInstallationStatus::CheckingForUpdate,
        );

        let release = zed::latest_github_release(
            LSP_RELEASE_REPO,
            zed::GithubReleaseOptions { require_assets: true, pre_release: false },
        )?;

        let (platform, arch) = zed::current_platform();
        let suffix = match (platform, arch) {
            (zed::Os::Mac, zed::Architecture::Aarch64) => "darwin-arm64",
            (zed::Os::Mac, zed::Architecture::X8664) => "darwin-amd64",
            (zed::Os::Linux, zed::Architecture::Aarch64) => "linux-arm64",
            (zed::Os::Linux, zed::Architecture::X8664) => "linux-amd64",
            (zed::Os::Windows, zed::Architecture::X8664) => "windows-amd64.exe",
            (os, arch) => return Err(format!("unsupported platform: {os:?}/{arch:?}").into()),
        };
        let asset_name = format!("d2-lsp-{suffix}");
        let asset = release
            .assets
            .iter()
            .find(|a| a.name == asset_name)
            .ok_or_else(|| format!("no asset named {asset_name} in release {}", release.version))?;

        let dest_dir = format!("d2-lsp-{}", release.version);
        let dest_path = format!(
            "{dest_dir}/d2-lsp{}",
            if matches!(platform, zed::Os::Windows) { ".exe" } else { "" }
        );

        zed::set_language_server_installation_status(
            lsp_id,
            &zed::LanguageServerInstallationStatus::Downloading,
        );

        // Task 11's release workflow uploads raw binaries (not tarballs/zips),
        // so Uncompressed is correct here. If the workflow ever switches to
        // compressed assets, change this in lockstep.
        zed::download_file(&asset.download_url, &dest_path, DownloadedFileType::Uncompressed)
            .map_err(|e| format!("download {}: {e}", asset.download_url))?;

        zed::make_file_executable(&dest_path)
            .map_err(|e| format!("chmod {dest_path}: {e}"))?;

        if let Ok(entries) = fs::read_dir(".") {
            for entry in entries.flatten() {
                let name = entry.file_name();
                let n = name.to_string_lossy();
                if n.starts_with("d2-lsp-") && n != dest_dir {
                    let _ = fs::remove_dir_all(entry.path());
                }
            }
        }

        self.cached_binary_path = Some(dest_path.clone());
        Ok(dest_path)
    }
}

zed::register_extension!(D2Extension);
