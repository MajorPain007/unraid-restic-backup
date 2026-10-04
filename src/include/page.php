<?php
$rbToken = isset($var['csrf_token']) ? $var['csrf_token'] : '';
$rbBase = '/plugins/restic.backup/assets';
?>
<link rel="stylesheet" href="<?php autov("$rbBase/app.css"); ?>">
<div id="rb-app" class="rb" data-csrf="<?=htmlspecialchars($rbToken, ENT_QUOTES)?>"
     data-api="/plugins/restic.backup/include/api.php">
  <div class="rb-boot">Loading&hellip;</div>
</div>
<?php foreach (array('core', 'jobs', 'repos', 'snapshots', 'activity', 'settings', 'consistency', 'preview', 'sizes', 'main') as $rbScript): ?>
<script src="<?php autov("$rbBase/js/$rbScript.js"); ?>"></script>
<?php endforeach; ?>
