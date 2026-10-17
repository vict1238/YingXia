# Developer asset generator. Uses the existing 映 character and app palette.
param([string]$Output = (Join-Path $PSScriptRoot '..\assets\windows\yingxia.ico'))
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$entries = @()
foreach ($size in @(32,48,64,256)) {
    $bitmap = [Drawing.Bitmap]::new($size,$size,[Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $graphics = [Drawing.Graphics]::FromImage($bitmap)
    $graphics.SmoothingMode = [Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $graphics.TextRenderingHint = [Drawing.Text.TextRenderingHint]::AntiAliasGridFit
    $graphics.Clear([Drawing.Color]::Transparent)
    $path = [Drawing.Drawing2D.GraphicsPath]::new()
    $inset = $size * 0.055
    $side = $size - 2*$inset
    $radius = $size * 0.23
    $path.AddArc($inset,$inset,$radius,$radius,180,90)
    $path.AddArc($inset+$side-$radius,$inset,$radius,$radius,270,90)
    $path.AddArc($inset+$side-$radius,$inset+$side-$radius,$radius,$radius,0,90)
    $path.AddArc($inset,$inset+$side-$radius,$radius,$radius,90,90)
    $path.CloseFigure()
    $background = [Drawing.SolidBrush]::new([Drawing.ColorTranslator]::FromHtml('#0f1418'))
    $accent = [Drawing.SolidBrush]::new([Drawing.ColorTranslator]::FromHtml('#8bdcc0'))
    $pen = [Drawing.Pen]::new($accent,[single]($size*0.026))
    $graphics.FillPath($background,$path)
    $graphics.DrawPath($pen,$path)
    $font = [Drawing.Font]::new('Microsoft YaHei UI',[single]($size*0.55),[Drawing.FontStyle]::Bold,[Drawing.GraphicsUnit]::Pixel)
    $format = [Drawing.StringFormat]::new()
    $format.Alignment = [Drawing.StringAlignment]::Center
    $format.LineAlignment = [Drawing.StringAlignment]::Center
    $rect = [Drawing.RectangleF]::new(0,-$size*0.016,$size,$size)
    $graphics.DrawString('映',$font,$accent,$rect,$format)
    $stream = [IO.MemoryStream]::new()
    $bitmap.Save($stream,[Drawing.Imaging.ImageFormat]::Png)
    $entries += @{size=$size;bytes=$stream.ToArray()}
    $stream.Dispose(); $format.Dispose(); $font.Dispose(); $pen.Dispose(); $accent.Dispose(); $background.Dispose(); $path.Dispose(); $graphics.Dispose(); $bitmap.Dispose()
}
$file = [IO.File]::Create([IO.Path]::GetFullPath($Output))
$writer = [IO.BinaryWriter]::new($file)
try {
    $writer.Write([uint16]0); $writer.Write([uint16]1); $writer.Write([uint16]$entries.Count)
    $offset = 6+16*$entries.Count
    foreach ($entry in $entries) {
        $dimension = if ($entry.size -eq 256) {0} else {$entry.size}
        $writer.Write([byte]$dimension); $writer.Write([byte]$dimension); $writer.Write([byte]0); $writer.Write([byte]0)
        $writer.Write([uint16]1); $writer.Write([uint16]32); $writer.Write([uint32]$entry.bytes.Length); $writer.Write([uint32]$offset)
        $offset += $entry.bytes.Length
    }
    foreach ($entry in $entries) {$writer.Write([byte[]]$entry.bytes)}
} finally { $writer.Dispose(); $file.Dispose() }
Get-FileHash -LiteralPath $Output -Algorithm SHA256
